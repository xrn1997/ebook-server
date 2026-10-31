package archtest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRulesCatchForbiddenEdge(t *testing.T) {
	pkgs := []Package{{ImportPath: "ebook-server/repository", Imports: []string{"ebook-server/service"}}}
	got := Violations(pkgs, Rules())
	if len(got) != 1 || got[0].Import != "ebook-server/service" {
		t.Fatalf("violations = %#v, want one service import", got)
	}
}

func TestRulesAllowForwardEdge(t *testing.T) {
	pkgs := []Package{{ImportPath: "ebook-server/handler", Imports: []string{"ebook-server/service", "github.com/gin-gonic/gin"}}}
	if got := Violations(pkgs, Rules()); len(got) != 0 {
		t.Fatalf("violations = %#v, want none", got)
	}
}

func TestRulePrefixBoundary(t *testing.T) {
	if (Rule{Prefix: "ebook-server/model"}).Matches("ebook-server/modeling") {
		t.Fatal("prefix matched a sibling package")
	}
	if !(Rule{Prefix: "ebook-server/model"}).Matches("ebook-server/model/sub") {
		t.Fatal("prefix did not match a subpackage")
	}
}

// expectedRules 是架构分层的规格说明：每个层必须禁止哪些层。
//
// 这张表对照 docs/architecture.md §依赖方向（分层与禁令的常驻事实源）逐条成立
// （migrations 一行是实现新增的：迁移文件同样不该反向依赖业务层）。
//
// 为什么要有这张表：Rules() 里的 Forbidden 列表一旦有笔误或漏项（比如把
// "service" 写成 "servcie"，或漏掉 "pkg"），真实依赖图里未必存在那条边，
// 门禁就会一直绿着——「我以为的约束」和「真正被强制的约束」就此分家，而且
// 没有任何测试会变红。这张表把分层写成可读的形式，让规则表与规格逐条比对。
//
// 架构真的变了（新增一层、调整边界）时，两处必须一起改；只改一处，另一处会红——
// 那正是要人确认的时刻。
var expectedRules = []struct {
	prefix    string
	forbidden []string
}{
	{"ebook-server/model", []string{"handler", "service", "repository", "router", "middleware", "config", "internal", "pkg"}},
	{"ebook-server/repository", []string{"handler", "service", "router", "middleware", "config", "internal"}},
	{"ebook-server/service", []string{"handler", "repository", "router", "middleware", "config", "internal"}},
	{"ebook-server/handler", []string{"repository", "router", "config", "internal"}},
	{"ebook-server/middleware", []string{"handler", "service", "repository", "router", "config", "internal"}},
	{"ebook-server/router", []string{"model", "service", "repository", "config"}},
	{"ebook-server/config", []string{"handler", "service", "repository", "router", "middleware", "internal", "pkg", "model"}},
	{"ebook-server/migrations", []string{"handler", "service", "repository", "router", "middleware", "config", "internal"}},
	{"ebook-server/internal", []string{"handler", "service", "repository", "router", "middleware"}},
	{"ebook-server/pkg", []string{"handler", "service", "repository", "router", "middleware", "internal"}},
	{"ebook-server/cmd", []string{"handler", "service", "repository", "router", "middleware", "internal"}},
}

// TestRuleTableMatchesSpecification 规则表必须与架构规格逐条一致。
//
// 少一条禁令 = 一条架构约束静默失效，而且没有任何测试会变红：真实依赖图里
// 未必存在那条边，门禁照样是绿的。这条测试是那张「不会静默失效」的保证。
func TestRuleTableMatchesSpecification(t *testing.T) {
	rules := Rules()
	if len(rules) != len(expectedRules) {
		t.Fatalf("规则表有 %d 条, 规格是 %d 条；新增或删除层时两处必须一起改", len(rules), len(expectedRules))
	}
	for i, want := range expectedRules {
		got := rules[i]
		if got.Prefix != want.prefix {
			t.Errorf("第 %d 条规则的 prefix = %q, 规格是 %q", i, got.Prefix, want.prefix)
			continue
		}
		if !sameSet(got.Forbidden, want.forbidden) {
			t.Errorf("规则 %s 的禁令 = %v, 规格是 %v", want.prefix, got.Forbidden, want.forbidden)
		}
	}
}

// TestEveryForbiddenEntryIsEnforced 规则表里的每一条禁令都必须真的挡得住。
//
// 规格比对保证「该禁的都禁了」；这条保证「禁了就是禁住了」——两件事缺一不可。
func TestEveryForbiddenEntryIsEnforced(t *testing.T) {
	for _, rule := range Rules() {
		for _, forbidden := range rule.Forbidden {
			t.Run(rule.Prefix+"->"+forbidden, func(t *testing.T) {
				importPath := modulePath + "/" + forbidden
				pkgs := []Package{{ImportPath: rule.Prefix + "/probe", Imports: []string{importPath}}}
				got := Violations(pkgs, Rules())
				if len(got) != 1 {
					t.Fatalf("violations = %#v, want exactly one (rule %s forbids %s)",
						got, rule.Prefix, forbidden)
				}
				if got[0].Import != importPath || got[0].Package != rule.Prefix+"/probe" {
					t.Fatalf("violation = %#v, want package %s importing %s",
						got[0], rule.Prefix+"/probe", importPath)
				}
			})
		}
	}
}

// sameSet 比较两个字符串集合（顺序无关）。
func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[string]int, len(a))
	for _, v := range a {
		set[v]++
	}
	for _, v := range b {
		set[v]--
		if set[v] < 0 {
			return false
		}
	}
	return true
}

// TestEveryRuleForbidsSomething 规则表里不许出现空禁令。
//
// 一条什么都不禁的规则看起来「有覆盖」，实际是装饰：包被它匹配上了，于是
// TestRulesCoverEveryPackage 认为它有归属，但它不约束任何东西。
func TestEveryRuleForbidsSomething(t *testing.T) {
	for _, rule := range Rules() {
		if len(rule.Forbidden) == 0 {
			t.Errorf("rule %s forbids nothing", rule.Prefix)
		}
	}
}

// TestMatchRulePicksLongestPrefix 命中的是最具体的规则，与规则表顺序无关。
//
// 「第一条命中」的实现下，一条更具体的子包规则只要排在通用规则后面就永远
// 匹配不到，而且不会有任何测试变红。
func TestMatchRulePicksLongestPrefix(t *testing.T) {
	rules := []Rule{
		{Prefix: "ebook-server/pkg", Forbidden: []string{"handler"}},
		{Prefix: "ebook-server/pkg/database", Forbidden: []string{"handler"}},
	}
	got, ok := matchRule("ebook-server/pkg/database", rules)
	if !ok {
		t.Fatal("no rule matched")
	}
	if got.Prefix != "ebook-server/pkg/database" {
		t.Fatalf("matched %s, want the longest prefix ebook-server/pkg/database", got.Prefix)
	}
}

// TestModulePathMatchesGoMod 规则表展开禁令时用的 module 前缀必须与 go.mod 一致。
//
// 前缀一旦对不上，所有禁令都指向不存在的包，整张规则表静默失效——门禁还是绿的。
func TestModulePathMatchesGoMod(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "go.mod"))
	if err != nil {
		t.Fatalf("读取 go.mod 失败: %v", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "module ") {
			continue
		}
		got := strings.TrimSpace(strings.TrimPrefix(line, "module "))
		if got != modulePath {
			t.Fatalf("go.mod 的 module 是 %q, archtest 的 modulePath 是 %q；"+
				"两者不一致时规则表会整体失效", got, modulePath)
		}
		return
	}
	t.Fatal("go.mod 里没有 module 声明")
}

func TestRulesCoverEveryPackage(t *testing.T) {
	pkgs, err := LoadPackages("../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range pkgs {
		// 根包（main 装配点）与 docs（Swagger 生成物）不属于任何一层。
		if p.ImportPath == "ebook-server" || p.ImportPath == "ebook-server/docs" {
			continue
		}
		if _, ok := matchRule(p.ImportPath, Rules()); !ok {
			t.Errorf("package %s is not covered by any arch rule", p.ImportPath)
		}
	}
}

func TestCurrentPackagesRespectLayers(t *testing.T) {
	pkgs, err := LoadPackages("../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range Violations(pkgs, Rules()) {
		t.Errorf("%s", v.String())
	}
}

func TestViolationMessageIsReadable(t *testing.T) {
	pkgs := []Package{{ImportPath: "ebook-server/service", Imports: []string{"ebook-server/handler"}}}
	got := Violations(pkgs, Rules())
	if len(got) != 1 || !strings.Contains(got[0].String(), "ebook-server/service") {
		t.Fatalf("violations = %#v", got)
	}
}
