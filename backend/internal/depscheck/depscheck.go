// Package depscheck 检查依赖清单的可移植性：前端锁文件里的每一条下载源，CI 与全新克隆都得够得着。
//
// 它随 go test ./... 运行，因此排在 make gates 的 test-race 阶段——早于第一次 `npm ci`。
// 放前端测试里没用：`npm ci` 装不上就跑不到那里，正是本次要拦的那一步。
package depscheck

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// lockfilePath 是被检查的锁文件（相对仓库根）。
const lockfilePath = "frontend/package-lock.json"

// Issue 是一条依赖问题；File 是相对仓库根的路径。
type Issue struct {
	File    string
	Message string
}

// publicRegistries 是 CI 与全新克隆都能解析到的下载源前缀。
//
// 判据是「CI 够不够得着」，不是「URL 看着像不像公网」：公网镜像（如
// mirrors.cloud.tencent.com）确实能访问，但放开到「任何 https host」就等于把内网
// 域名与拼错的域名一起放过。要新增一类源（git 依赖、公司代理）就显式加前缀，并在
// docs/development.md 的前端小节写明为什么。
var publicRegistries = []string{"https://registry.npmjs.org/"}

// lockfile 是 npm lockfileVersion 3 的最小投影。
//
// 只取 packages 的键与 resolved：version 与 integrity 由 npm 自己校验，这里管的是
// 下载地址在别的网络上存不存在。
type lockfile struct {
	Packages map[string]struct {
		Resolved string `json:"resolved"`
	} `json:"packages"`
}

// Check 扫描 root 下的前端锁文件，返回全部不可移植的下载源。
func Check(root string) []Issue {
	path := filepath.Join(root, lockfilePath)
	data, err := os.ReadFile(path)
	if err != nil {
		return []Issue{{File: lockfilePath, Message: "missing lock file: " + err.Error()}}
	}

	var lock lockfile
	if err := json.Unmarshal(data, &lock); err != nil {
		return []Issue{{File: lockfilePath, Message: "cannot parse lock file: " + err.Error()}}
	}
	if len(lock.Packages) == 0 {
		// 空 packages 会让逐条比较退化成「无问题」——版本号换了形态或被截断时，
		// 门禁不能因此静默绿。
		return []Issue{{File: lockfilePath, Message: "lock file has no packages entries; check lockfileVersion"}}
	}

	keys := make([]string, 0, len(lock.Packages))
	for key := range lock.Packages {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var issues []Issue
	for _, key := range keys {
		resolved := lock.Packages[key].Resolved
		if resolved == "" {
			// 根条目与 link: 依赖没有下载地址。
			continue
		}
		if reason := violation(resolved); reason != "" {
			issues = append(issues, Issue{
				File:    lockfilePath,
				Message: fmt.Sprintf("%s: %s (%s)", key, resolved, reason),
			})
		}
	}
	return issues
}

// violation 判断一条 resolved 下载地址是否不可移植，返回理由；可移植时返回空串。
func violation(resolved string) string {
	for _, prefix := range publicRegistries {
		if strings.HasPrefix(resolved, prefix) {
			return ""
		}
	}
	return "不在公网源清单里，CI 装不上；改走 https://registry.npmjs.org/ 或把源加进 publicRegistries"
}
