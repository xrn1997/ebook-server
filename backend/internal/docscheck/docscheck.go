// Package docscheck 检查常驻文档的活性：路径、链接、行号、根 AGENTS 预算与 ADR 元数据。
//
// 它随 go test ./... 运行；扫描范围是常驻文档与 docs/adr/*.md，不含 docs/superpowers/**
// 的过程工件。检查刻意保守：跳过命令、通配、占位符与构建产物，避免制造大量误报。
package docscheck

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// maxAgentsChars 是根 AGENTS.md 的字符预算（ADR-0020）。
const maxAgentsChars = 8000

// Issue 是一条文档问题；File 是相对仓库根的路径。
type Issue struct {
	File    string
	Message string
}

var (
	// lineRefRe 匹配「文件名:行号」形态的引用。
	//
	// 扩展名必须以字母开头：这样 IPv4 的最后一段（127.0.0.1 的 "1"）不会
	// 被当成扩展名，地址里的「主机:端口」也就不会被误报成行号。URL 里的
	// host:port 由 findLineRefs 另行排除。
	lineRefRe  = regexp.MustCompile(`\b[\w./-]+\.[A-Za-z][A-Za-z0-9]*:\d+\b`)
	backtickRe = regexp.MustCompile("`([^`\n]+)`")
	linkRe     = regexp.MustCompile(`\[[^\]]*\]\(([^)]+)\)`)
	adrFileRe  = regexp.MustCompile(`^(\d{4})-.*\.md$`)
	adrRowRe   = regexp.MustCompile(`^\|\s*(\d{4})\s*\|`)
	// adrRowStatusRe 抓索引行的编号与状态列（前三列固定是编号/标题/状态）。
	adrRowStatusRe = regexp.MustCompile(`^\|\s*(\d{4})\s*\|[^|\n]*\|\s*([^|\n]*)\s*\|`)
	statusRe       = regexp.MustCompile(`(?m)^Status:\s*(.+)$`)
	anchorRe       = regexp.MustCompile(`(?m)^锚点：`)
	partialRe      = regexp.MustCompile(`(?m)^Superseded in part by:\s*(\d{4})`)
	supersededRe   = regexp.MustCompile(`^superseded by \d{4}$`)
)

var permanentDocs = []string{
	"AGENTS.md",
	"CONTEXT.md",
	"README.md",
	"TESTING.md",
	"CONTRIBUTING.md",
	"docs/architecture.md",
	"docs/development.md",
	"docs/AGENTS.md",
}

var requiredDocs = append(append([]string{}, permanentDocs...),
	"docs/adr/README.md",
	"docs/adr/ADR-FORMAT.md",
)

// pathPrefixes 是「点名即须存在」的路径前缀范围。desktop/ 已随 ADR-0024 整体删除，
// 但清单刻意保留它：文档点名 `desktop/...` 必须当场变红（或标（已删）豁免），
// 与 topLevelFiles 保留 Dockerfile 同一口径——删掉的名字恰恰是最需要门禁的一瞬。
var pathPrefixes = []string{
	"backend/",
	"desktop/",
	"docs/",
	"frontend/",
	"sql/",
}

var topLevelFiles = map[string]bool{
	"AGENTS.md":       true,
	"CONTEXT.md":      true,
	"README.md":       true,
	"TESTING.md":      true,
	"CONTRIBUTING.md": true,
	"config.yaml":     true,
	"Makefile":        true,
	// 顶层还有几个「没有可扩展名」的文件名，文档里会以裸名引用。这份清单点的是
	// **可能被引用的名字**，不是当前存在的文件：删掉一个顶层文件时，文档里那句
	// 引用必须当场变红——Dockerfile 就是这样漏掉过（ADR 还在描述它的构建编排）。
	"Dockerfile":   true,
	"LICENSE":      true,
	".gitignore":   true,
	".env.example": true,
}

// bareFileExtensions 认出「这个裸 token 是个文件名」。
//
// 文档里的路径大多带目录前缀（走 pathPrefixes 检查），但也有 `migrate.go`、
// `0001_init.sql` 这种以文件名为锚的写法；它们不带前缀，光靠 pathPrefixes 就查不到。
// 带这些后缀的裸名会被当作文件引用，存在性按「仓库里有没有同名文件」判断，
// 所以子目录里的真实文件不会因为不在根目录而被误判。
var bareFileExtensions = map[string]bool{
	".md": true, ".go": true, ".yaml": true, ".yml": true, ".json": true,
	".sql": true, ".ts": true, ".js": true, ".vue": true, ".css": true,
	".html": true, ".sh": true, ".mod": true, ".sum": true, ".txt": true,
	".rc": true, ".example": true,
}

// 构建产物与生成物不要求入库，因此文档提到它们时不做存在性检查。
// 目录按路径边界匹配，生成物文件名按字面前缀匹配（常带平台/架构后缀，
// 如 rsrc_windows_amd64.syso）；两种都不能退化成任意前缀，否则
// `backend/internal/admin/webhooks.go` 这类真实文件会被静默放过。
var skipPathDirs = []string{
	"frontend/dist",
	"backend/internal/admin/web",
	"build",
	"logs",
	"uploads",
	"node_modules",
}

var skipPathPrefixes = []string{
	"backend/rsrc_windows_",
	"coverage.out",
	"ebook.db",
	// 过程工件只住工作区、整目录不入库（docs/AGENTS.md §4）。文档会按规则点名这个
	// 目录，但全新克隆里它并不存在——不跳过的话，docscheck 在开发者机器上绿、
	// 在干净检出上红。
	"docs/superpowers/",
}

// Check 扫描 root 下的常驻文档与 ADR，返回全部问题。
func Check(root string) []Issue {
	var issues []Issue

	for _, file := range requiredDocs {
		if _, err := os.Stat(filepath.Join(root, file)); err != nil {
			issues = append(issues, Issue{File: file, Message: "missing file"})
		}
	}

	if data, err := os.ReadFile(filepath.Join(root, "AGENTS.md")); err == nil {
		if n := utf8.RuneCountInString(string(data)); n > maxAgentsChars {
			issues = append(issues, Issue{
				File:    "AGENTS.md",
				Message: fmt.Sprintf("AGENTS.md is %d characters; budget is %d", n, maxAgentsChars),
			})
		}
	}

	names := repoFileNames(root)
	for _, file := range scanDocs(root) {
		data, err := os.ReadFile(filepath.Join(root, file))
		if err != nil {
			continue
		}
		issues = append(issues, checkDoc(root, file, string(data), names)...)
	}
	return append(issues, checkADRs(root)...)
}

// scanDocs 返回常驻文档与 ADR 目录全部 Markdown 文件的固定顺序清单。
//
// docs/adr 下的每个 .md 都要扫：索引（README.md）与格式规范（ADR-FORMAT.md）
// 是链接与路径最密的两份常驻文档，只扫 NNNN-*.md 等于把最需要检查的两份漏掉。
func scanDocs(root string) []string {
	files := append([]string{}, permanentDocs...)
	entries, err := os.ReadDir(filepath.Join(root, "docs/adr"))
	if err != nil {
		return files
	}
	var adrNames []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		adrNames = append(adrNames, e.Name())
	}
	sort.Strings(adrNames)
	for _, name := range adrNames {
		files = append(files, "docs/adr/"+name)
	}
	return files
}

// checkDoc 检查单篇文档的行号、反引号路径与相对链接。
func checkDoc(root, file, text string, names map[string]bool) []Issue {
	var issues []Issue

	for _, ref := range findLineRefs(text) {
		issues = append(issues, Issue{File: file, Message: "line reference: " + ref})
	}

	// 被取代的 ADR 保留正文当历史（见 docs/adr/ADR-FORMAT.md），它点名的文件往往
	// 已随那次撤销一起删除。对这类文档不做路径存在性检查，否则门禁会逼着改写
	// 历史记录；相对链接照查——取代关系指向的那篇 ADR 必须还在。
	superseded := isSupersededADR(text)

	for _, loc := range backtickRe.FindAllStringSubmatchIndex(text, -1) {
		if superseded {
			break
		}
		// 「（已删）」紧跟在路径之后 = 显式豁免（docs/AGENTS.md §3.4）：
		// 记录删除决策或历史时可以点名已删路径，但必须自己写明它已删。
		if markedDeleted(text, loc[1]) {
			continue
		}
		token, ok := candidatePath(text[loc[2]:loc[3]])
		if !ok {
			continue
		}
		if !pathExists(root, token, names) {
			issues = append(issues, Issue{File: file, Message: "missing path: " + token})
		}
	}

	base := filepath.Dir(filepath.Join(root, file))
	for _, m := range linkRe.FindAllStringSubmatch(text, -1) {
		target, ok := candidateLink(m[1])
		if !ok {
			continue
		}
		if _, err := os.Stat(filepath.Join(base, target)); err != nil {
			issues = append(issues, Issue{File: file, Message: "broken relative link: " + target})
		}
	}

	return issues
}

// findLineRefs 找出文档里的「文件名:行号」形态。
//
// 两种东西长得像，必须分开：
//   - 行号引用（config.yaml:12）——文档规则禁止，它会随代码漂移；
//   - 主机:端口（127.0.0.1:9091、example.com:8080）——地址，不是行号。
//
// IPv4 由正则本身排除（扩展名必须以字母开头，127.0.0.1 的最后一段是数字）；
// URL 里的 host:port 由本函数排除：它总在 scheme 之后。
func findLineRefs(text string) []string {
	var refs []string
	for _, loc := range lineRefRe.FindAllStringIndex(text, -1) {
		if isInsideURL(text, loc[0]) {
			continue
		}
		refs = append(refs, text[loc[0]:loc[1]])
	}
	return refs
}

// isInsideURL 判断匹配起点是否落在 URL 的 host 部分。
//
// 判定方式：往前找最近的 "://"，且它离匹配点不超过 8 个字符——scheme 最长不过
// "https"（5 个字符）加上分隔符，超过了就说明 "://" 属于别的东西。
func isInsideURL(text string, start int) bool {
	prefix := text[:start]
	i := strings.LastIndex(prefix, "://")
	return i >= 0 && len(prefix)-i <= 8
}

// pathExists 解析一个被文档点名的路径。
//
// 带目录前缀的按仓库根解析；裸文件名允许出现在仓库任何位置——文档习惯以文件名为锚
// （`migrate.go` 指的是 backend/pkg/database/migrate.go），只按根目录判断会把
// 正确写法一律判成死引用。
//
// 含 `..` 段的引用一律视为不存在：filepath.Join 会把它折成仓库外的路径，
// `backend/../../outside.go` 恰好存在时就绕过了检查。
func pathExists(root, token string, names map[string]bool) bool {
	for _, segment := range strings.Split(token, "/") {
		if segment == ".." {
			return false
		}
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(token))); err == nil {
		return true
	}
	if !strings.Contains(token, "/") {
		return names[token]
	}
	return false
}

// repoFileNames 建立仓库文件基名索引，供裸文件名检查使用。
//
// 依赖与产物目录整体跳过：node_modules 里几万个文件既让索引失去意义，也会把
// `index.js` 这类随便一个词都判成「存在」。
func repoFileNames(root string) map[string]bool {
	names := make(map[string]bool, 256)
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skippedDir(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		names[d.Name()] = true
		return nil
	})
	return names
}

// skippedDir 版本库元数据、依赖与构建产物目录——它们不参与基名索引。
func skippedDir(name string) bool {
	if name == ".git" || name == "node_modules" {
		return true
	}
	for _, dir := range skipPathDirs {
		// skipPathDirs 写的是仓库相对路径（frontend/dist），这里只比对最后一段
		if base := filepath.Base(filepath.ToSlash(dir)); base == name {
			return true
		}
	}
	return false
}

// statusKind 把 Status 值折成可比的两类：accepted 与 superseded。
//
// 索引的「状态」列允许追加范围说明（`accepted；…部分被 0024 取代`），ADR 的 Status
// 也允许 `superseded by NNNN`；两边都只比较前导词，否则「部分取代」的写法永远对不上。
func statusKind(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	if i := strings.IndexAny(status, "；;"); i >= 0 {
		status = strings.ToLower(strings.TrimSpace(status[:i]))
	}
	if strings.HasPrefix(status, "superseded") {
		return "superseded"
	}
	return status
}

// isSupersededADR 判断这篇 ADR 是否整篇被取代（Status: superseded by NNNN）。
//
// 被取代的 ADR 按 docs/adr/ADR-FORMAT.md 保留正文当历史，其中点名的文件多数已随那次
// 撤销一起删除；对它们做存在性检查只会逼人改写历史记录，而不是修当前事实。
func isSupersededADR(text string) bool {
	m := statusRe.FindStringSubmatch(text)
	if m == nil {
		return false
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(m[1])), "superseded")
}

// candidatePath 判断反引号 token 是否是一个应当存在的仓库路径。
func candidatePath(raw string) (string, bool) {
	token := strings.TrimRight(strings.TrimSpace(raw), ".,;:")
	if token == "" || strings.Contains(token, "...") {
		return "", false
	}
	// 命名模板不是路径：ADR-FORMAT 用 `NNNN-slug.md`、development.md 用
	// `NNNN_slug.sql` 表达编号规则，这样的名字永远不会有对应的文件。
	// 判据取「连续三个以上 N」——README.md 这类真名不受影响。
	if strings.Contains(token, "NNN") {
		return "", false
	}
	if strings.ContainsAny(token, " \t\n<>*|$#?={}") {
		return "", false
	}
	if strings.HasPrefix(token, "http://") || strings.HasPrefix(token, "https://") || strings.HasPrefix(token, "mailto:") {
		return "", false
	}
	if strings.HasPrefix(token, "/") || strings.HasPrefix(token, "../") || strings.HasPrefix(token, "./") {
		return "", false
	}
	if topLevelFiles[token] {
		return token, true
	}
	if !strings.Contains(token, "/") {
		// 裸写的文件名（`migrate.go`、`0001_init.sql`）也是文件引用；带目录前缀的
		// 交给下面的规则。没有已知扩展名的裸词（`admin`、`A0400`）不是路径。
		if ext := filepath.Ext(token); ext != "" && bareFileExtensions[strings.ToLower(ext)] {
			return token, true
		}
		return "", false
	}

	hasPrefix := false
	for _, p := range pathPrefixes {
		if strings.HasPrefix(token, p) {
			hasPrefix = true
			break
		}
	}
	if !hasPrefix {
		return "", false
	}

	for _, p := range skipPathPrefixes {
		if strings.HasPrefix(token, p) {
			return "", false
		}
	}
	for _, d := range skipPathDirs {
		if token == d || strings.HasPrefix(token, d+"/") {
			return "", false
		}
	}
	return token, true
}

// deletedMarker 是已删路径的显式标注（docs/AGENTS.md §3.4）：紧接在路径之后，
// 中间允许空白。豁免是写出来的，不靠上下文猜——句中出现「删除」二字不豁免。
const deletedMarker = "（已删）"

// markedDeleted 判断以 end 结束的反引号路径是否带（已删）标注。
func markedDeleted(text string, end int) bool {
	return strings.HasPrefix(strings.TrimLeft(text[end:], " \t"), deletedMarker)
}

// candidateLink 判断 Markdown 链接是否是需要检查的相对路径。
func candidateLink(raw string) (string, bool) {
	target := strings.TrimSpace(raw)
	target = strings.Trim(target, "<>")
	if i := strings.IndexAny(target, " \t"); i >= 0 {
		target = target[:i]
	}
	if target == "" || strings.HasPrefix(target, "#") {
		return "", false
	}
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") ||
		strings.HasPrefix(target, "mailto:") || strings.HasPrefix(target, "/") {
		return "", false
	}
	if i := strings.Index(target, "#"); i >= 0 {
		target = target[:i]
	}
	if target == "" {
		return "", false
	}
	return target, true
}

// checkADRs 检查索引与文件的一一对应、Status、部分取代与锚点。
func checkADRs(root string) []Issue {
	dir := filepath.Join(root, "docs/adr")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []Issue{{File: "docs/adr", Message: "missing directory"}}
	}

	// 按文件名收集，再按编号归组：编号重复时两个文件都要检查，不能只查最后一个
	// （用 map[编号]文件名 存的话，后一个会盖掉前一个）。
	var adrNames []string
	byNumber := map[string][]string{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := adrFileRe.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		adrNames = append(adrNames, e.Name())
		byNumber[m[1]] = append(byNumber[m[1]], e.Name())
	}
	sort.Strings(adrNames)

	var issues []Issue
	for _, n := range sortedNumbers(byNumber) {
		if len(byNumber[n]) > 1 {
			issues = append(issues, Issue{
				File:    "docs/adr",
				Message: "duplicate ADR number " + n + " used by " + strings.Join(byNumber[n], ", "),
			})
		}
	}

	indexData, err := os.ReadFile(filepath.Join(dir, "README.md"))
	rowStatus := map[string]string{}
	if err != nil {
		issues = append(issues, Issue{File: "docs/adr/README.md", Message: "missing file"})
	} else {
		rows := map[string]int{}
		for _, line := range strings.Split(string(indexData), "\n") {
			if m := adrRowRe.FindStringSubmatch(line); m != nil {
				rows[m[1]]++
				if s := adrRowStatusRe.FindStringSubmatch(line); s != nil {
					if _, ok := rowStatus[m[1]]; !ok {
						rowStatus[m[1]] = s[2]
					}
				}
			}
		}
		for _, n := range sortedNumbers(byNumber) {
			if rows[n] == 0 {
				issues = append(issues, Issue{File: "docs/adr/README.md", Message: "ADR index missing " + n})
			}
		}
		for _, n := range sortedRowNumbers(rows) {
			if _, ok := byNumber[n]; !ok {
				issues = append(issues, Issue{File: "docs/adr/README.md", Message: "ADR index has unknown " + n})
			}
		}
		// 一篇 ADR 在索引里出现恰好一次：重复行会让「索引 = 文件集」的对应关系失真。
		for _, n := range sortedRowNumbers(rows) {
			if rows[n] > 1 {
				issues = append(issues, Issue{
					File:    "docs/adr/README.md",
					Message: fmt.Sprintf("ADR index lists %s %d times; each ADR must appear exactly once", n, rows[n]),
				})
			}
		}
	}

	for _, name := range adrNames {
		file := "docs/adr/" + name
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			issues = append(issues, Issue{File: file, Message: "missing file"})
			continue
		}
		text := string(data)

		statusMatch := statusRe.FindStringSubmatch(text)
		if statusMatch == nil {
			issues = append(issues, Issue{File: file, Message: "missing Status:"})
		} else {
			status := strings.TrimSpace(statusMatch[1])
			if status != "accepted" && !supersededRe.MatchString(status) {
				issues = append(issues, Issue{File: file, Message: "invalid Status: " + status})
			}
			// 索引的状态列必须与 ADR 的 Status 同源：索引写着 accepted、文件已被取代，
			// 读者会照着索引去找一份不存在的当前决定。两边都只比较前导词。
			if fileMatch := adrFileRe.FindStringSubmatch(name); fileMatch != nil {
				if cell, ok := rowStatus[fileMatch[1]]; ok && statusKind(cell) != statusKind(status) {
					issues = append(issues, Issue{
						File: "docs/adr/README.md",
						Message: fmt.Sprintf("ADR index status %q disagrees with %s Status: %s",
							strings.TrimSpace(cell), name, status),
					})
				}
			}
			if partial := partialRe.FindStringSubmatch(text); partial != nil {
				if status != "accepted" {
					issues = append(issues, Issue{File: file, Message: "Superseded in part by requires Status: accepted"})
				}
				if _, ok := byNumber[partial[1]]; !ok {
					issues = append(issues, Issue{File: file, Message: "Superseded in part by references missing ADR " + partial[1]})
				}
			}
		}

		if !anchorRe.MatchString(text) {
			issues = append(issues, Issue{File: file, Message: "missing anchor: 锚点："})
		}
	}

	return issues
}

// sortedNumbers 返回编号集合的有序列表。
func sortedNumbers(byNumber map[string][]string) []string {
	out := make([]string, 0, len(byNumber))
	for n := range byNumber {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func sortedRowNumbers(rows map[string]int) []string {
	out := make([]string, 0, len(rows))
	for n := range rows {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
