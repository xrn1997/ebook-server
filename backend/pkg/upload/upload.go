// Package upload 提供头像文件存储与公开访问（ADR-0011）。
//
// 职责：校验（格式/大小）、随机命名、写盘、按 URL 解析删除、拼接公开 URL。
// 只服务公开资源（头像），不承载私有文件——仅头像子目录经 /uploads/avatar 静态公开。
package upload

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ebook-server/model"
)

// 头像上传约束（ADR-0011，与 android-ebook 需求单 3.3 对齐）。
const (
	// MaxAvatarBytes 头像文件大小上限：5MB。
	MaxAvatarBytes = 5 << 20
	// AvatarSubdir 头像子目录名（upload.dir 之下）。
	AvatarSubdir = "avatar"
	// AvatarURLPrefix 头像的公开访问前缀：/uploads/avatar。
	//
	// 前缀里的 /uploads/ 只是 URL 空间，与磁盘上的 upload.dir 没有对应关系；真正的
	// 挂载目标是 AvatarDir（<upload.dir>/avatar）。挂载点刻意收窄到头像子目录而不是
	// 整个 upload.dir：静态服务会把根目录下的每个文件都公开出去，而服务端的工作
	// 目录（也就是相对 upload.dir 解析的地方）同时放着 config.yaml、.env 与数据库
	// 文件。本服务唯一会写的公开资源就是头像，多挂一层目录只是把兄弟文件递到陌生人
	// 手里。gin 的 Static 不接受带尾斜杠的路径，这个值刚好合用。
	AvatarURLPrefix = "/uploads/" + AvatarSubdir
)

// avatarExts 允许的图片扩展名白名单（与 DetectContentType 无关，纯按文件名后缀）。
var avatarExts = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".webp": true,
}

// Store 头像文件存储。root 为上传根目录（config upload.dir），
// publicBase 为钉死的公开来源（config server.public_base_url，可为空）。
type Store struct {
	root       string
	avatarDir  string
	publicBase string
}

// New 创建存储实例，并确保头像目录存在（启动即建，省去首次上传失败）。
// 目录可能在运行期被外部清理（git clean 等），SaveAvatar 写盘前会再兜底一次。
//
// publicBaseURL 允许带尾斜杠：那种写法与不带的必须给出同一个 URL。
func New(root, publicBaseURL string) *Store {
	s := &Store{
		root:       root,
		avatarDir:  filepath.Join(root, AvatarSubdir),
		publicBase: strings.TrimRight(strings.TrimSpace(publicBaseURL), "/"),
	}
	_ = os.MkdirAll(s.avatarDir, 0o755)
	return s
}

// AvatarDir 返回头像文件的实际目录（供测试与运维确认落盘位置）。
func (s *Store) AvatarDir() string {
	return s.avatarDir
}

// SaveAvatar 保存头像文件，返回文件名（不含目录）。
//
// 校验扩展名与大小；文件名用「时间戳 + 随机 hex」，避免碰撞与路径注入。
func (s *Store) SaveAvatar(header *multipart.FileHeader) (string, error) {
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !avatarExts[ext] {
		return "", model.ErrInvalidImage
	}
	if header.Size > MaxAvatarBytes || header.Size <= 0 {
		return "", model.ErrInvalidImage
	}

	src, err := header.Open()
	if err != nil {
		return "", fmt.Errorf("open upload file: %w", err)
	}
	defer src.Close()

	name := newAvatarName(ext)
	dstPath := filepath.Join(s.avatarDir, name)
	// 写盘前兜底确保目录存在（目录可能在运行期被清理）；MkdirAll 幂等，已存在时零开销。
	// 与 New() 的启动建目录互补：目录缺失时此处自动重建，不再依赖重启。
	if err := os.MkdirAll(s.avatarDir, 0o755); err != nil {
		return "", fmt.Errorf("ensure avatar dir: %w", err)
	}
	dst, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", fmt.Errorf("create avatar file: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		// 写盘失败清理半成品，避免残留垃圾文件
		_ = os.Remove(dstPath)
		return "", fmt.Errorf("write avatar file: %w", err)
	}
	return name, nil
}

// DeleteAvatar 删除头像文件。
//
// avatarURL 若是本服务 uploads/avatar 下的文件则删除，否则视为外部 URL 不做任何
// 操作（不删用户手工填的外部图床文件）。删除失败返回 error，由调用方记日志。
func (s *Store) DeleteAvatar(avatarURL string) error {
	if avatarURL == "" {
		return nil
	}
	name, ok := s.resolveAvatarName(avatarURL)
	if !ok {
		return nil // 外部 URL，不属于本服务
	}
	if err := os.Remove(filepath.Join(s.avatarDir, name)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// PublicURL 拼出头像的公开访问 URL（ADR-0011）。
//
// 三档，按可信度从高到低：
//  1. 配置给了 publicBase：只用它，请求 Host 完全不参与。伪造 Host 把外链写进数据库、
//     再经评论列表分发给其他用户，是自托管下唯一能被利用的通道，钉死来源即关掉它；
//  2. 没配置、且 Host 是可信形状（DNS 名或 IP，可带端口）：按 scheme://Host 拼——
//     局域网直连零配置就能用，代价见 README 的残余风险说明；
//  3. 其余（Host 带路径/scheme/凭证/空白，或端口越界）：只回站内相对路径，
//     绝不把来源不可信的 origin 回显进响应。
func (s *Store) PublicURL(scheme, host, name string) string {
	path := AvatarURLPrefix + "/" + name
	if s.publicBase != "" {
		return s.publicBase + path
	}
	if !isRequestHostValid(host) {
		return path
	}
	return scheme + "://" + host + path
}

// isRequestHostValid 判断请求 Host 能否原样拼进 URL。
func isRequestHostValid(host string) bool {
	if host == "" || strings.ContainsAny(host, `/?\@ `) {
		return false
	}
	name := host
	if strings.Contains(host, ":") {
		var port string
		var err error
		name, port, err = net.SplitHostPort(host)
		if err != nil {
			return false
		}
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return false
		}
	}
	if net.ParseIP(name) != nil {
		return true
	}
	return isDNSLike(name)
}

// isDNSLike 主机名字形：点分标签，每标签字母数字与连字符、首尾不是连字符。
// 收下 `localhost` 与 `192.168.0.156.nip.io` 这类，拒掉 `a b`、`-x`、`a..b`。
func isDNSLike(name string) bool {
	if name == "" || len(name) > 253 {
		return false
	}
	for _, label := range strings.Split(strings.TrimSuffix(name, "."), ".") {
		if label == "" || len(label) > 63 {
			return false
		}
		for i := 0; i < len(label); i++ {
			c := label[i]
			alnum := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
			if !alnum && c != '-' {
				return false
			}
			if c == '-' && (i == 0 || i == len(label)-1) {
				return false
			}
		}
	}
	return true
}

// resolveAvatarName 从 URL 解析出属于本服务头像目录的文件名。
//
// 仅当 URL 路径以 /uploads/avatar/ 开头、剩余部分是合法文件名（无路径分隔符）
// 时返回 ok=true。其余（外部 URL、非法路径）返回 ok=false。
func (s *Store) resolveAvatarName(avatarURL string) (string, bool) {
	u, err := url.Parse(avatarURL)
	if err != nil {
		return "", false
	}
	prefix := AvatarURLPrefix + "/"
	rest := strings.TrimPrefix(u.Path, prefix)
	if rest == u.Path {
		return "", false // 不在本服务头像路径下
	}
	// 防目录穿越：必须是真正的文件名。`.`/`..` 是无分隔符的路径段，能通过
	// Base(rest) == rest 的检查，却会让 os.Remove 删到头像目录（或 uploads 根）；
	// 末尾的点/空格在 Windows 的文件名归一化里会被忽略，`a.png.` 会删到 `a.png`。
	// 引用计数按文件名比较，这类变体一旦漏算就会误删别的账号仍引用的文件，
	// 所以在删除入口一并拒绝。
	if rest == "" || rest == "." || rest == ".." ||
		strings.TrimRight(rest, ". ") != rest ||
		strings.ContainsAny(rest, `/\`) || filepath.Base(rest) != rest {
		return "", false
	}
	return rest, true
}

// newAvatarName 生成唯一文件名：<unix毫秒>_<16字节随机hex>.<ext>。
func newAvatarName(ext string) string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%d_%s%s", time.Now().UnixMilli(), hex.EncodeToString(b[:]), ext)
}
