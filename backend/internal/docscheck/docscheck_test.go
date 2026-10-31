package docscheck

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRepositoryDocsAreLive(t *testing.T) {
	issues := Check(repoRoot(t))
	if len(issues) > 0 {
		t.Fatalf("docscheck found %d issue(s):\n%s", len(issues), formatIssues(issues))
	}
}

// TestSkipsBuildArtifacts 产物与生成物不做存在性检查。
//
// `backend/rsrc_windows_amd64.syso` 只在 Windows 构建时出现：非 Windows 主机上
// 文档提到它，仓库里并没有这个文件，门禁不能因此变红。
func TestSkipsBuildArtifacts(t *testing.T) {
	root := t.TempDir()
	for _, f := range requiredDocs {
		p := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("# ok\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	readme := "# ok\n\n`build/ebook-server.exe` `frontend/dist/assets/index.js` " +
		"`backend/rsrc_windows_amd64.syso` `backend/internal/admin/web/index.html`\n"
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte(readme), 0o644); err != nil {
		t.Fatal(err)
	}
	if issues := Check(root); len(issues) > 0 {
		t.Fatalf("build artifacts should be skipped:\n%s", formatIssues(issues))
	}
}

// TestSkipsWorkspaceOnlyProcessDocs docs/superpowers/ 是只住工作区的过程工件目录，
// 整目录被 .gitignore 排除。文档按规则点名它时，干净检出里没有这个目录——
// 门禁不能因此在开发者机器与全新克隆上给出两个答案。
func TestSkipsWorkspaceOnlyProcessDocs(t *testing.T) {
	root := t.TempDir()
	for _, f := range requiredDocs {
		p := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("# ok\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	readme := "# ok\n\n过程工件只留在工作区的 `docs/superpowers/`，整目录不入库。\n"
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte(readme), 0o644); err != nil {
		t.Fatal(err)
	}
	if issues := Check(root); len(issues) > 0 {
		t.Fatalf("工作区专属目录不该参与存在性检查:\n%s", formatIssues(issues))
	}
}

// TestBareFilenamesResolveAnywhereInRepo 裸写的文件名只要在仓库任意目录里存在就不算失效。
//
// 文档习惯以文件名为锚（`migrate.go`、`0001_init.sql`），它们都不在仓库根。存在性检查
// 若只按根目录解析，加入裸文件名规则会把满仓的正确写法一律判成死引用——门禁一旦
// 天天误报，下一次真死引用就没人看了。
func TestBareFilenamesResolveAnywhereInRepo(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range requiredDocs {
		write(f, "# ok\n")
	}
	write("README.md", "# ok\n\n`migrate.go` 与 `0001_init.sql` 与 `docscheck.go` 与 `NNNN-slug.md`\n")
	write("backend/pkg/database/migrate.go", "package database\n")
	write("backend/migrations/0001_init.sql", "-- baseline\n")
	write("backend/internal/docscheck/docscheck.go", "package docscheck\n")

	if issues := Check(root); len(issues) > 0 {
		t.Fatalf("仓库里存在的裸文件名与命名模板都不该被判失效:\n%s", formatIssues(issues))
	}
}

// TestSupersededADRKeepsHistoricalPaths 整篇被取代的 ADR 保留正文当历史，
// 其中的文件已随那次撤销删除；路径存在性检查对它豁免，否则门禁会逼着改写历史。
func TestSupersededADRKeepsHistoricalPaths(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range requiredDocs {
		write(f, "# ok\n")
	}
	write("README.md", "# ok\n")
	write("docs/adr/README.md", "| 编号 | 标题 | 状态 |\n| --- | --- | --- |\n| 0001 | a | superseded by 0002 |\n| 0002 | b | accepted |\n")
	write("docs/adr/0001-a.md", "# a\n\nStatus: superseded by 0002\n\n桌面端曾以 `sidecar.ts` 复用后端，配置见 `electron-builder.yml`。\n\n锚点：`README.md`。\n")
	write("docs/adr/0002-b.md", "# b\n\nStatus: accepted\n\n锚点：`README.md`。\n")

	if issues := Check(root); len(issues) > 0 {
		t.Fatalf("被取代的 ADR 不应为历史路径变红:\n%s", formatIssues(issues))
	}
}

// TestDeletedMarkerExemptsHistoricalPaths 记录删除决策或历史时点名已删路径，
// 必须紧接标（已删）（docs/AGENTS.md §3.4）。标注是显式豁免，不靠上下文猜——
// 同一路径不标就变红（TestEachRuleFires 的「已删顶层目录下的路径失效」）。
func TestDeletedMarkerExemptsHistoricalPaths(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range requiredDocs {
		write(f, "# ok\n")
	}
	write("README.md", "# ok\n\n桌面端 `desktop/src/main/config.ts`（已删）的 `SCHEMA` 曾是三份契约之一。\n")

	if issues := Check(root); len(issues) > 0 {
		t.Fatalf("带（已删）标注的历史路径不该变红:\n%s", formatIssues(issues))
	}
}

func TestChecksCatchProblems(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("AGENTS.md", strings.Repeat("a", maxAgentsChars+1))
	for _, f := range []string{
		"CONTEXT.md", "TESTING.md", "CONTRIBUTING.md",
		"docs/architecture.md", "docs/development.md", "docs/AGENTS.md",
		"docs/adr/ADR-FORMAT.md",
	} {
		write(f, "# ok\n")
	}
	write("README.md", "# ok\n\n`backend/missing.go` and bad.go:12 and [x](missing.md)\n")
	write("docs/adr/README.md", "| 编号 | 标题 | 状态 |\n| --- | --- | --- |\n| 0002 | b | accepted |\n")
	write("docs/adr/0001-a.md", "# a\n\nStatus: accepted\n")
	write("docs/adr/0002-b.md", "# b\n\nStatus: bogus\n\n锚点：`README.md`。\n")

	joined := formatIssues(Check(root))
	for _, want := range []string{
		"AGENTS.md",
		"missing path",
		"line reference",
		"broken relative link",
		"missing anchor",
		"invalid Status",
		"ADR index",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("issues missing %q:\n%s", want, joined)
		}
	}
}

// TestEachRuleFires 逐条规则都要能自己变红。
//
// 合并在一起断言关键词，只能证明「某处有东西被报了」：AGENTS.md 预算那条可能
// 是被路径问题顺带命中的，"missing Status:" 那条可能根本没触发。规则各有各的
// 触发条件，必须一条一条单独验。
func TestEachRuleFires(t *testing.T) {
	cases := []struct {
		name string
		// files 是要写进临时仓库的文件；缺失的必需文件由 fixture 补齐。
		files map[string]string
		// omit 是「故意不写」的必需文档，用来触发 missing file 那条。
		omit []string
		want string
	}{
		{
			name: "缺少必需文档",
			omit: []string{"docs/architecture.md"},
			want: "missing file",
		},
		{
			name:  "AGENTS.md 超出字符预算",
			files: map[string]string{"AGENTS.md": strings.Repeat("a", maxAgentsChars+1)},
			want:  "budget",
		},
		{
			name:  "反引号里的路径不存在",
			files: map[string]string{"README.md": "# ok\n\n`backend/nope/nope.go`\n"},
			want:  "missing path",
		},
		{
			// 顶层文件名不带目录前缀，曾逃过存在性检查：删掉 Dockerfile 之后，
			// ADR 里那句「树里还有 Dockerfile」一直绿着——恰恰是最需要门禁的一瞬间。
			name:  "顶层裸文件名已不存在",
			files: map[string]string{"README.md": "# ok\n\n容器构建由 `Dockerfile` 承担\n"},
			want:  "missing path: Dockerfile",
		},
		{
			// 已删的顶层成员（desktop/，ADR-0024）同理：非豁免文档点名它下面的文件
			// 必须当场变红，路径带目录前缀也不能逃过检查。
			name:  "已删顶层目录下的路径失效",
			files: map[string]string{"README.md": "# ok\n\n`desktop/src/main/config.ts` 定义了 `SCHEMA`\n"},
			want:  "missing path: desktop/src/main/config.ts",
		},
		{
			name:  "带扩展名的裸文件名不存在",
			files: map[string]string{"README.md": "# ok\n\n基线见 `init.sql`\n"},
			want:  "missing path: init.sql",
		},
		{
			// `..` 段会把 filepath.Join 折到仓库外；恰好存在的外部文件不能让引用通过。
			name:  "含 .. 的路径引用",
			files: map[string]string{"README.md": "# ok\n\n`backend/../../outside.go`\n"},
			want:  "missing path: backend/../../outside.go",
		},
		{
			name:  "行号引用",
			files: map[string]string{"README.md": "# ok\n\n见 config.yaml:12 与 sql/init.sql:30\n"},
			want:  "line reference",
		},
		{
			name:  "相对链接失效",
			files: map[string]string{"README.md": "# ok\n\n[x](nope.md)\n"},
			want:  "broken relative link",
		},
		{
			name: "ADR 缺 Status",
			files: map[string]string{
				"docs/adr/README.md": "| 0001 | a | accepted |\n",
				"docs/adr/0001-a.md": "# a\n\n锚点：`README.md`。\n",
			},
			want: "missing Status:",
		},
		{
			name: "Status 取值非法",
			files: map[string]string{
				"docs/adr/README.md": "| 0001 | a | accepted |\n",
				"docs/adr/0001-a.md": "# a\n\nStatus: bogus\n\n锚点：`README.md`。\n",
			},
			want: "invalid Status:",
		},
		{
			name: "缺锚点",
			files: map[string]string{
				"docs/adr/README.md": "| 0001 | a | accepted |\n",
				"docs/adr/0001-a.md": "# a\n\nStatus: accepted\n",
			},
			want: "missing anchor:",
		},
		{
			name: "部分取代要求 accepted",
			files: map[string]string{
				"docs/adr/README.md": "| 0001 | a | accepted |\n| 0002 | b | accepted |\n",
				"docs/adr/0001-a.md": "# a\n\nStatus: superseded by 0002\n\nSuperseded in part by: 0002（范围）\n\n锚点：`README.md`。\n",
				"docs/adr/0002-b.md": "# b\n\nStatus: accepted\n\n锚点：`README.md`。\n",
			},
			want: "Superseded in part by requires Status: accepted",
		},
		{
			name: "部分取代指向不存在的 ADR",
			files: map[string]string{
				"docs/adr/README.md": "| 0001 | a | accepted |\n",
				"docs/adr/0001-a.md": "# a\n\nStatus: accepted\n\nSuperseded in part by: 0099（范围）\n\n锚点：`README.md`。\n",
			},
			want: "Superseded in part by references missing ADR 0099",
		},
		{
			name: "索引缺条目",
			files: map[string]string{
				"docs/adr/README.md": "| 编号 | 标题 | 状态 |\n",
				"docs/adr/0001-a.md": "# a\n\nStatus: accepted\n\n锚点：`README.md`。\n",
			},
			want: "ADR index missing 0001",
		},
		{
			name: "索引有条目但没有对应文件",
			files: map[string]string{
				"docs/adr/README.md": "| 0001 | a | accepted |\n| 0007 | g | accepted |\n",
				"docs/adr/0001-a.md": "# a\n\nStatus: accepted\n\n锚点：`README.md`。\n",
			},
			want: "ADR index has unknown 0007",
		},
		{
			name: "同一编号两个文件",
			files: map[string]string{
				"docs/adr/README.md": "| 0001 | a | accepted |\n",
				"docs/adr/0001-a.md": "# a\n\nStatus: accepted\n\n锚点：`README.md`。\n",
				"docs/adr/0001-b.md": "# b\n\nStatus: accepted\n\n锚点：`README.md`。\n",
			},
			want: "duplicate ADR number 0001",
		},
		{
			name: "索引里同一 ADR 出现两次",
			files: map[string]string{
				"docs/adr/README.md": "| 0001 | a | accepted |\n| 0001 | a | accepted |\n",
				"docs/adr/0001-a.md": "# a\n\nStatus: accepted\n\n锚点：`README.md`。\n",
			},
			want: "each ADR must appear exactly once",
		},
		{
			// 索引只数编号时，一行写 accepted、文件已被取代也能通过；读者会照着
			// 索引去找一份不存在的当前决定。
			name: "索引状态与 ADR Status 不一致",
			files: map[string]string{
				"docs/adr/README.md": "| 0001 | a | accepted |\n| 0002 | b | accepted |\n",
				"docs/adr/0001-a.md": "# a\n\nStatus: superseded by 0002\n\n锚点：`README.md`。\n",
				"docs/adr/0002-b.md": "# b\n\nStatus: accepted\n\n锚点：`README.md`。\n",
			},
			want: "ADR index status",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			write := func(rel, body string) {
				t.Helper()
				p := filepath.Join(root, rel)
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			// 先补齐必需文档，让被测的那条规则是唯一的失败原因。
			for _, f := range requiredDocs {
				if _, ok := tc.files[f]; ok {
					continue
				}
				if contains(tc.omit, f) {
					continue
				}
				write(f, "# ok\n")
			}
			for rel, body := range tc.files {
				write(rel, body)
			}

			joined := formatIssues(Check(root))
			if !strings.Contains(joined, tc.want) {
				t.Fatalf("issues missing %q:\n%s", tc.want, joined)
			}
		})
	}
}

// TestDocsAdrIndexAndFormatAreScanned 索引与格式规范也在扫描范围内。
//
// 这两份是链接与路径最密的常驻文档；只扫 NNNN-*.md 等于把它们漏掉，而它们
// 出问题（死链、失效路径）时不会有任何提示。
func TestDocsAdrIndexAndFormatAreScanned(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range requiredDocs {
		write(f, "# ok\n")
	}
	write("docs/adr/README.md", "# index\n\n`backend/does/not/exist.go`\n")
	write("docs/adr/ADR-FORMAT.md", "# format\n\n见 config.yaml:12\n")

	joined := formatIssues(Check(root))
	for _, want := range []string{"docs/adr/README.md", "missing path", "docs/adr/ADR-FORMAT.md", "line reference"} {
		if !strings.Contains(joined, want) {
			t.Errorf("issues missing %q:\n%s", want, joined)
		}
	}
}

// TestHostPortIsNotALineReference 地址不能被当成行号引用。
//
// 文档里到处是 127.0.0.1:9091 这类「主机:端口」，把它们报成行号引用会让门禁
// 变成噪声，久了就没人看了。
func TestHostPortIsNotALineReference(t *testing.T) {
	for _, text := range []string{
		"后台监听 127.0.0.1:9091",
		"公开 API 绑在 0.0.0.0:9090",
		"见 http://example.com:8080/health",
		"访问 https://example.com:8443/admin",
	} {
		if got := findLineRefs(text); len(got) != 0 {
			t.Errorf("findLineRefs(%q) = %v, want none", text, got)
		}
	}
	for _, text := range []string{
		"见 config.yaml:12",
		"见 sql/init.sql:30",
		"见 backend/docs/swagger.json:5",
	} {
		if got := findLineRefs(text); len(got) != 1 {
			t.Errorf("findLineRefs(%q) = %v, want one", text, got)
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

// contains 判断列表里是否有该元素。
func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func formatIssues(issues []Issue) string {
	var b strings.Builder
	for _, issue := range issues {
		fmt.Fprintf(&b, "%s: %s\n", issue.File, issue.Message)
	}
	return b.String()
}
