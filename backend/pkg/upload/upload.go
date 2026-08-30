// Package upload 提供头像文件存储与公开访问（ADR-0011）。
//
// 职责：校验（格式/大小）、随机命名、写盘、按 URL 解析删除、拼接公开 URL。
// 只服务公开资源（头像），不承载私有文件——目录通过 /uploads/* 静态公开。
package upload

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// 头像上传约束（ADR-0011，与 android-ebook 需求单 3.3 对齐）。
const (
	// MaxAvatarBytes 头像文件大小上限：5MB。
	MaxAvatarBytes = 5 << 20
	// AvatarSubdir 头像子目录名（upload.dir 之下）。
	AvatarSubdir = "avatar"
	// StaticPrefix 公开访问前缀（静态服务挂载点，main.go 注册 /uploads/*）。
	StaticPrefix = "/uploads/"
)

// avatarExts 允许的图片扩展名白名单（与 DetectContentType 无关，纯按文件名后缀）。
var avatarExts = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".webp": true,
}

// ErrInvalidImage 文件格式或大小不符合头像约束。
var ErrInvalidImage = errors.New("仅支持 jpg/png/webp 图片，且大小不超过 5MB")

// Store 头像文件存储。root 为上传根目录（config upload.dir）。
type Store struct {
	root      string
	avatarDir string
}

// New 创建存储实例，并确保头像目录存在（启动即建，省去首次上传失败）。
// 目录可能在运行期被外部清理（git clean 等），SaveAvatar 写盘前会再兜底一次。
func New(root string) *Store {
	s := &Store{
		root:      root,
		avatarDir: filepath.Join(root, AvatarSubdir),
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
		return "", ErrInvalidImage
	}
	if header.Size > MaxAvatarBytes || header.Size <= 0 {
		return "", ErrInvalidImage
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

// PublicURL 拼接头像的公开访问 URL（ADR-0011：拼接请求 Host 的绝对 URL）。
//
// 自托管场景无固定域名，用请求 Host（如 192.168.0.156:9090）动态拼接，
// 客户端拿到即可直接加载。scheme 由调用方按 TLS 有无传入（http/https）。
func (s *Store) PublicURL(scheme, host, name string) string {
	return scheme + "://" + host + StaticPrefix + AvatarSubdir + "/" + name
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
	prefix := StaticPrefix + AvatarSubdir + "/"
	rest := strings.TrimPrefix(u.Path, prefix)
	if rest == u.Path {
		return "", false // 不在本服务头像路径下
	}
	// 防目录穿越：文件名必须是纯文件名
	if rest == "" || strings.ContainsAny(rest, `/\`) || filepath.Base(rest) != rest {
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
