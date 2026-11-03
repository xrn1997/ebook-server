package depscheck

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFrontendLockfileIsPortable(t *testing.T) {
	issues := Check(repoRoot(t))
	if len(issues) > 0 {
		t.Fatalf("depscheck found %d issue(s):\n%s", len(issues), formatIssues(issues))
	}
}

// TestFlagsUnreachableSources 锁文件里指向内网源的 `resolved` 必须当场变红。
//
// `mirrors.tencentyun.com` 只在腾讯云 VPC 内解析得到：开发机上装得动，CI 与新克隆
// 上 `npm ci` 直接 ENOTFOUND。npm 把 `resolved` 按**当时生效的 registry** 写进锁文件，
// 所以一次带内网镜像的 `npm i -D` 就会把这批 URL 提交进库——门禁不能靠装过依赖的机器。
func TestFlagsUnreachableSources(t *testing.T) {
	for _, tc := range []struct {
		name     string
		resolved string
		wantOK   bool
	}{
		{"公网 registry", "https://registry.npmjs.org/acorn/-/acorn-8.19.0.tgz", true},
		{"内网镜像 host", "http://mirrors.tencentyun.com/npm/acorn/-/acorn-8.19.0.tgz", false},
		{"公网镜像但不在清单", "https://mirrors.cloud.tencent.com/npm/acorn/-/acorn-8.19.0.tgz", false},
		{"公司内网 https", "https://npm.corp.example.com/acorn/-/acorn-8.19.0.tgz", false},
		{"公网 host 但明文传输", "http://registry.npmjs.org/acorn/-/acorn-8.19.0.tgz", false},
		{"git 源", "git+ssh://git@github.com/acornjs/acorn.git", false},
		{"本地 file 源", "file:../vendor/acorn", false},
	} {
		got := violation(tc.resolved) == ""
		if got != tc.wantOK {
			t.Errorf("violation(%q) 判定为 %v，期望可移植性 %v（原因：%s）",
				tc.resolved, got, tc.wantOK, tc.name)
		}
	}
}

// TestCheckReportsPackageKey 问题条目要带包名，改的人不必自己在几千行 JSON 里找。
func TestCheckReportsPackageKey(t *testing.T) {
	root := writeLockfile(t, `{
  "name": "ebook-admin",
  "lockfileVersion": 3,
  "packages": {
    "": {
      "name": "ebook-admin"
    },
    "node_modules/acorn": {
      "version": "8.19.0",
      "resolved": "http://mirrors.tencentyun.com/npm/acorn/-/acorn-8.19.0.tgz",
      "integrity": "sha512-AAAA",
      "dev": true
    },
    "node_modules/picomatch": {
      "version": "4.0.7",
      "resolved": "https://registry.npmjs.org/picomatch/-/picomatch-4.0.7.tgz",
      "integrity": "sha512-BBBB"
    }
  }
}`)

	issues := Check(root)
	if len(issues) != 1 {
		t.Fatalf("只应报内网源那一条，实得 %d 条:\n%s", len(issues), formatIssues(issues))
	}
	one := issues[0]
	if !strings.Contains(one.Message, "node_modules/acorn") {
		t.Errorf("问题条目应含包键名，实得 %q", one.Message)
	}
	if !strings.Contains(one.Message, "mirrors.tencentyun.com") {
		t.Errorf("问题条目应含被拒 URL，实得 %q", one.Message)
	}
	if !strings.Contains(one.File, "package-lock.json") {
		t.Errorf("问题应指向锁文件，实得 %q", one.File)
	}
}

// TestMissingAndUnparseableLockfileAreIssues 读不到锁文件不能当成通过。
//
// 「文件不在」与「JSON 坏了」都让检查失去意义；静默绿一次，下一次内网 URL 就跟着
// commit 进来了。前端整体删除时这条会红，那时按当次决定删掉这个门禁包。
func TestMissingAndUnparseableLockfileAreIssues(t *testing.T) {
	root := t.TempDir()
	if issues := Check(root); len(issues) != 1 || !strings.Contains(issues[0].Message, "missing") {
		t.Fatalf("缺失锁文件应报 missing:\n%s", formatIssues(issues))
	}

	bad := t.TempDir()
	if err := os.MkdirAll(filepath.Join(bad, "frontend"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, lockfilePath), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if issues := Check(bad); len(issues) != 1 || !strings.Contains(issues[0].Message, "parse") {
		t.Fatalf("坏锁文件应报 parse 失败:\n%s", formatIssues(issues))
	}
}

// writeLockfile 建一个只含锁文件的临时仓库根。
func writeLockfile(t *testing.T, body string) string {
	t.Helper()
	root := t.TempDir()
	p := filepath.Join(root, lockfilePath)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func formatIssues(issues []Issue) string {
	var b strings.Builder
	for _, issue := range issues {
		b.WriteString(issue.File + ": " + issue.Message + "\n")
	}
	return b.String()
}
