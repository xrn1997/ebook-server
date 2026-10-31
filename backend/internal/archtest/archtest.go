// Package archtest 检查 Go 包的依赖方向。
//
// 规则由 Rules 给出，LoadPackages 用 go list 读取生产 import（不含测试依赖），
// Violations 返回违反方向的边。测试包本身随 go test ./... 运行。
package archtest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// Package 是 go list 输出的最小投影。
type Package struct {
	ImportPath string
	Imports    []string
}

// Rule 是一条包前缀上的依赖禁令。
type Rule struct {
	Prefix    string
	Forbidden []string
}

// Violation 是一条被规则挡下的 import。
type Violation struct {
	Package string
	Import  string
	Rule    string
}

// String 让测试与 CI 输出能直接定位包、import 与命中的规则。
func (v Violation) String() string {
	return fmt.Sprintf("%s imports forbidden %s (rule %s)", v.Package, v.Import, v.Rule)
}

// Matches 判断包路径是否属于本规则；按路径段匹配，避免 model 命中 modeling。
func (r Rule) Matches(importPath string) bool {
	return importPath == r.Prefix || strings.HasPrefix(importPath, r.Prefix+"/")
}

// Forbids 判断一条 import 是否被本规则禁止。
func (r Rule) Forbids(importPath string) bool {
	for _, f := range r.Forbidden {
		full := modulePath + "/" + f
		if importPath == full || strings.HasPrefix(importPath, full+"/") {
			return true
		}
	}
	return false
}

// modulePath 是本仓库的 Go module 路径。
//
// 规则表里的禁令写的是包名（"service"），展开时拼上 module 前缀。这个常量一旦
// 与 go.mod 对不上，所有禁令都会指向不存在的包——整张规则表静默失效。TestModulePathMatchesGoMod
// 把这个前提钉住。
const modulePath = "ebook-server"

// Rules 返回当前已成立的依赖方向。
//
// 只钉住现在真实成立的方向；新增包若没有命中任何规则，由
// TestRulesCoverEveryPackage 报出来，逼作者显式决定它属于哪一层。
func Rules() []Rule {
	return []Rule{
		{Prefix: "ebook-server/model", Forbidden: []string{"handler", "service", "repository", "router", "middleware", "config", "internal", "pkg"}},
		{Prefix: "ebook-server/repository", Forbidden: []string{"handler", "service", "router", "middleware", "config", "internal"}},
		{Prefix: "ebook-server/service", Forbidden: []string{"handler", "repository", "router", "middleware", "config", "internal"}},
		{Prefix: "ebook-server/handler", Forbidden: []string{"repository", "router", "config", "internal"}},
		{Prefix: "ebook-server/middleware", Forbidden: []string{"handler", "service", "repository", "router", "config", "internal"}},
		{Prefix: "ebook-server/router", Forbidden: []string{"model", "service", "repository", "config"}},
		{Prefix: "ebook-server/config", Forbidden: []string{"handler", "service", "repository", "router", "middleware", "internal", "pkg", "model"}},
		{Prefix: "ebook-server/migrations", Forbidden: []string{"handler", "service", "repository", "router", "middleware", "config", "internal"}},
		{Prefix: "ebook-server/internal", Forbidden: []string{"handler", "service", "repository", "router", "middleware"}},
		{Prefix: "ebook-server/pkg", Forbidden: []string{"handler", "service", "repository", "router", "middleware", "internal"}},
		{Prefix: "ebook-server/cmd", Forbidden: []string{"handler", "service", "repository", "router", "middleware", "internal"}},
	}
}

// matchRule 返回命中最具体的规则（最长前缀）。
//
// 用最长前缀而不是「第一条命中」：规则表因此与书写顺序无关，将来加一条更具体的
// 子包规则不必记得把它排在前面，否则它永远匹配不到。
func matchRule(importPath string, rules []Rule) (Rule, bool) {
	var best Rule
	bestLen := -1
	for _, r := range rules {
		if !r.Matches(importPath) {
			continue
		}
		if len(r.Prefix) > bestLen {
			best, bestLen = r, len(r.Prefix)
		}
	}
	return best, bestLen >= 0
}

// Violations 返回所有违反规则的边。
func Violations(pkgs []Package, rules []Rule) []Violation {
	var out []Violation
	for _, p := range pkgs {
		rule, ok := matchRule(p.ImportPath, rules)
		if !ok {
			continue
		}
		for _, imp := range p.Imports {
			if rule.Forbids(imp) {
				out = append(out, Violation{Package: p.ImportPath, Import: imp, Rule: rule.Prefix})
			}
		}
	}
	return out
}

// LoadPackages 读取 dir 下所有生产包的 import 面。
//
// stdout 与 stderr 必须分开取：go list 会把「go: downloading …」「go: warning: …」
// 之类的进度与警告写到 stderr，CombinedOutput 把它们拼在 JSON 前面，解码器就会
// 在合法输出上报错——模块缓存冷启动或依赖有告警时，这条门禁会以与分层无关的
// 原因变红。
func LoadPackages(dir string) ([]Package, error) {
	cmd := exec.Command("go", "list", "-json", "./...")
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	dec := json.NewDecoder(bytes.NewReader(out))
	var pkgs []Package
	for dec.More() {
		var p Package
		if err := dec.Decode(&p); err != nil {
			return nil, fmt.Errorf("decode go list output: %w", err)
		}
		pkgs = append(pkgs, p)
	}
	return pkgs, nil
}
