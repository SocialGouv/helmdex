package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"helmdex/internal/helmutil"
	"helmdex/internal/yamlchart"

	"gopkg.in/yaml.v3"

	"github.com/charmbracelet/lipgloss"
)

// chartArtifacts are the chart-level artifacts used for upgrade diffs.
//
// - Schema is values.schema.json (may be empty).
// - Values is values.yaml (may be empty).
type chartArtifacts struct {
	Schema string
	Values string
}

// loadChartArtifactsBestEffort loads values.schema.json and values.yaml for a given dependency+version.
//
// This is a best-effort, multi-tier loader (vendored -> cached .tgz -> show cache -> pull .tgz -> helm show values).
// Schema is not available via `helm show`, so Schema may remain empty.
func loadChartArtifactsBestEffort(ctx context.Context, repoRoot, instancePath string, dep yamlchart.Dependency, version string, allowVendored bool) (chartArtifacts, error) {
	version = strings.TrimSpace(version)
	if version == "" {
		return chartArtifacts{}, fmt.Errorf("version is required")
	}
	// Use a copy so we can reuse helmutil cache keys by version.
	d := dep
	d.Version = version

	// 0) Vendored chart dir (only when explicitly allowed).
	if allowVendored && strings.TrimSpace(instancePath) != "" {
		base := filepath.Join(instancePath, "charts", d.Name)
		if st, err := os.Stat(base); err == nil && st.IsDir() {
			schemaPath := filepath.Join(base, "values.schema.json")
			valuesPath := filepath.Join(base, "values.yaml")
			out := chartArtifacts{}
			if b, err := os.ReadFile(schemaPath); err == nil {
				out.Schema = string(b)
				if strings.TrimSpace(out.Schema) != "" {
					_ = helmutil.WriteShowCache(repoRoot, d.Repository, d.Name, d.Version, helmutil.ShowKindSchema, out.Schema)
				}
			}
			if b, err := os.ReadFile(valuesPath); err == nil {
				out.Values = string(b)
				if strings.TrimSpace(out.Values) != "" {
					_ = helmutil.WriteShowCache(repoRoot, d.Repository, d.Name, d.Version, helmutil.ShowKindValues, out.Values)
				}
			}
			if strings.TrimSpace(out.Schema) != "" || strings.TrimSpace(out.Values) != "" {
				return out, nil
			}
		}
	}

	// 1) Cached chart archive (.tgz).
	if tgzPath, ok := helmutil.FindCachedChartArchive(repoRoot, d.Repository, d.Name, d.Version); ok {
		_, values, schema, err := helmutil.ReadChartArchiveFilesWithSchema(tgzPath)
		if err == nil {
			out := chartArtifacts{Schema: schema, Values: values}
			if strings.TrimSpace(out.Schema) != "" {
				_ = helmutil.WriteShowCache(repoRoot, d.Repository, d.Name, d.Version, helmutil.ShowKindSchema, out.Schema)
			}
			if strings.TrimSpace(out.Values) != "" {
				_ = helmutil.WriteShowCache(repoRoot, d.Repository, d.Name, d.Version, helmutil.ShowKindValues, out.Values)
			}
			if strings.TrimSpace(out.Schema) != "" || strings.TrimSpace(out.Values) != "" {
				return out, nil
			}
		}
	}

	// 2) helmdex show cache.
	out := chartArtifacts{}
	if s, ok, err := helmutil.ReadShowCache(repoRoot, d.Repository, d.Name, d.Version, helmutil.ShowKindSchema); err != nil {
		return chartArtifacts{}, err
	} else if ok {
		out.Schema = s
	}
	if s, ok, err := helmutil.ReadShowCache(repoRoot, d.Repository, d.Name, d.Version, helmutil.ShowKindValues); err != nil {
		return chartArtifacts{}, err
	} else if ok {
		out.Values = s
	}
	if strings.TrimSpace(out.Schema) != "" || strings.TrimSpace(out.Values) != "" {
		return out, nil
	}

	// 3) Pull chart archive and read.
	env := helmutil.EnvForRepoURL(repoRoot, d.Repository)
	ctx2, cancel2 := context.WithTimeout(ctx, 60*time.Second)
	defer cancel2()
	if tgzPath, err := helmutil.PullChartArchive(ctx2, env, d.Repository, d.Name, d.Version); err == nil {
		_, values, schema, err2 := helmutil.ReadChartArchiveFilesWithSchema(tgzPath)
		if err2 == nil {
			out = chartArtifacts{Schema: schema, Values: values}
			if strings.TrimSpace(out.Schema) != "" {
				_ = helmutil.WriteShowCache(repoRoot, d.Repository, d.Name, d.Version, helmutil.ShowKindSchema, out.Schema)
			}
			if strings.TrimSpace(out.Values) != "" {
				_ = helmutil.WriteShowCache(repoRoot, d.Repository, d.Name, d.Version, helmutil.ShowKindValues, out.Values)
			}
			if strings.TrimSpace(out.Schema) != "" || strings.TrimSpace(out.Values) != "" {
				return out, nil
			}
		}
	}

	// 4) Last resort: helm show values (values only).
	ctx3, cancel3 := context.WithTimeout(ctx, 60*time.Second)
	defer cancel3()
	if strings.HasPrefix(d.Repository, "oci://") {
		ref, err := helmutil.OCIChartRef(d.Repository, d.Name)
		if err != nil {
			return chartArtifacts{}, err
		}
		values, err := helmutil.ShowValues(ctx3, env, ref, d.Version)
		if err != nil {
			return chartArtifacts{}, err
		}
		_ = helmutil.WriteShowCache(repoRoot, d.Repository, d.Name, d.Version, helmutil.ShowKindValues, values)
		return chartArtifacts{Schema: "", Values: values}, nil
	}
	repoName := helmutil.RepoNameForURL(d.Repository)
	_ = helmutil.RepoAdd(ctx3, env, repoName, d.Repository)
	ref := repoName + "/" + d.Name
	values, err := helmutil.ShowValuesBestEffort(ctx3, env, ref, d.Version, 24*time.Hour)
	if err != nil {
		return chartArtifacts{}, err
	}
	_ = helmutil.WriteShowCache(repoRoot, d.Repository, d.Name, d.Version, helmutil.ShowKindValues, values)
	return chartArtifacts{Schema: "", Values: values}, nil
}

type diffCounts struct {
	Added   int
	Removed int
	Changed int
}

type diffLine struct {
	Path string
	Kind string // "+", "-", "~"
	Text string
}

// diffRow represents a single logical row for side-by-side rendering.
//
// - Path is the key (sorted, stable).
// - Old/New are rendered scalar/short JSON/YAML strings.
// - Kind is "add" | "del" | "chg".
type diffRow struct {
	Path string
	Old  string
	New  string
	Kind string
}

type styledSpan struct {
	Text  string
	Style lipgloss.Style
}

func spanTextLen(s styledSpan) int {
	return len([]rune(s.Text))
}

func renderSpans(spans []styledSpan) string {
	out := strings.Builder{}
	for _, sp := range spans {
		if sp.Text == "" {
			continue
		}
		out.WriteString(sp.Style.Render(sp.Text))
	}
	return out.String()
}

func wrapSpans(spans []styledSpan, width int) [][]styledSpan {
	// Wrap by rune width while preserving styles. Does NOT try to be word-aware;
	// callers should keep spans reasonably sized.
	if width <= 0 {
		return [][]styledSpan{spans}
	}
	lines := [][]styledSpan{}
	cur := []styledSpan{}
	curW := 0
	pushLine := func() {
		lines = append(lines, cur)
		cur = []styledSpan{}
		curW = 0
	}
	for _, sp := range spans {
		if sp.Text == "" {
			continue
		}
		r := []rune(sp.Text)
		for len(r) > 0 {
			if curW >= width {
				pushLine()
			}
			space := width - curW
			if space <= 0 {
				pushLine()
				space = width
			}
			n := len(r)
			if n > space {
				n = space
			}
			seg := string(r[:n])
			cur = append(cur, styledSpan{Text: seg, Style: sp.Style})
			curW += n
			r = r[n:]
		}
	}
	pushLine()
	// Avoid returning a trailing empty line.
	if len(lines) > 0 {
		last := lines[len(lines)-1]
		empty := true
		for _, sp := range last {
			if strings.TrimSpace(sp.Text) != "" {
				empty = false
				break
			}
		}
		if empty {
			lines = lines[:len(lines)-1]
		}
	}
	if len(lines) == 0 {
		return [][]styledSpan{{}}
	}
	return lines
}

func commonPrefixLenRunes(a, b string) int {
	ra := []rune(a)
	rb := []rune(b)
	n := len(ra)
	if len(rb) < n {
		n = len(rb)
	}
	i := 0
	for i < n && ra[i] == rb[i] {
		i++
	}
	return i
}

func commonSuffixLenRunes(a, b string, prefix int) int {
	ra := []rune(a)
	rb := []rune(b)
	ai := len(ra) - 1
	bi := len(rb) - 1
	s := 0
	for ai >= prefix && bi >= prefix && ra[ai] == rb[bi] {
		s++
		ai--
		bi--
	}
	return s
}

func splitValueForIntraline(s string) (prefix, value string, ok bool) {
	// We only intraline-highlight the value portion after ": ".
	idx := strings.Index(s, ": ")
	if idx < 0 {
		return s, "", false
	}
	return s[:idx+2], s[idx+2:], true
}

func intralineSpans(prefix, aVal, bVal string, baseStyle lipgloss.Style, intraStyle lipgloss.Style) (spans []styledSpan) {
	// Compute (prefix + common + highlighted-diff + common-suffix)
	// If there's no overlap, highlight the full value.
	p := commonPrefixLenRunes(aVal, bVal)
	s := commonSuffixLenRunes(aVal, bVal, p)
	ra := []rune(aVal)
	aMid := string(ra[p : len(ra)-s])
	commonPre := string(ra[:p])
	commonSuf := ""
	if s > 0 {
		commonSuf = string(ra[len(ra)-s:])
	}

	// For the side we're rendering, highlight its mid portion. If mid is empty,
	// don't apply intra highlight.
	spans = append(spans, styledSpan{Text: prefix, Style: baseStyle})
	spans = append(spans, styledSpan{Text: commonPre, Style: baseStyle})
	if strings.TrimSpace(aMid) != "" {
		spans = append(spans, styledSpan{Text: aMid, Style: intraStyle})
	}
	spans = append(spans, styledSpan{Text: commonSuf, Style: baseStyle})
	return spans
}

func renderUnifiedRows(rows []diffRow, width int, wrap bool) string {
	// Build a git-ish unified diff from rows.
	out := strings.Builder{}
	out.WriteString(styleDiffHdr.Render("diff --git a/values b/values"))
	out.WriteString("\n")
	out.WriteString(styleDiffHdr.Render("--- a/values"))
	out.WriteString("\n")
	out.WriteString(styleDiffHdr.Render("+++ b/values"))
	out.WriteString("\n")

	for _, r := range rows {
		isChanged := strings.TrimSpace(r.Old) != "" && strings.TrimSpace(r.New) != ""

		if strings.TrimSpace(r.Old) != "" {
			plain := "- " + r.Path + ": " + r.Old
			if isChanged {
				pfx, oldVal, ok := splitValueForIntraline(plain)
				if ok {
					newPlain := "+ " + r.Path + ": " + r.New
					_, newVal, _ := splitValueForIntraline(newPlain)
					spans := intralineSpans(pfx, oldVal, newVal, styleDiffDel, styleDiffDelIntra)
					lines := wrapSpans(spans, func() int {
						if wrap {
							return width
						}
						return 0
					}())
					for _, ln := range lines {
						out.WriteString(renderSpans(ln))
						out.WriteString("\n")
					}
					continue
				}
			}
			for _, seg := range wrapLine(plain, width, wrap) {
				out.WriteString(styleDiffDel.Render(seg))
				out.WriteString("\n")
			}
		}
		if strings.TrimSpace(r.New) != "" {
			plain := "+ " + r.Path + ": " + r.New
			if isChanged {
				pfx, newVal, ok := splitValueForIntraline(plain)
				if ok {
					oldPlain := "- " + r.Path + ": " + r.Old
					_, oldVal, _ := splitValueForIntraline(oldPlain)
					// Highlight the *new* mid portion; compute prefix/suffix based on old/new.
					p := commonPrefixLenRunes(oldVal, newVal)
					s := commonSuffixLenRunes(oldVal, newVal, p)
					rn := []rune(newVal)
					commonPre := string(rn[:p])
					commonSuf := ""
					if s > 0 {
						commonSuf = string(rn[len(rn)-s:])
					}
					mid := string(rn[p : len(rn)-s])
					spans := []styledSpan{{Text: pfx, Style: styleDiffAdd}, {Text: commonPre, Style: styleDiffAdd}}
					if strings.TrimSpace(mid) != "" {
						spans = append(spans, styledSpan{Text: mid, Style: styleDiffAddIntra})
					}
					spans = append(spans, styledSpan{Text: commonSuf, Style: styleDiffAdd})
					lines := wrapSpans(spans, func() int {
						if wrap {
							return width
						}
						return 0
					}())
					for _, ln := range lines {
						out.WriteString(renderSpans(ln))
						out.WriteString("\n")
					}
					continue
				}
			}
			for _, seg := range wrapLine(plain, width, wrap) {
				out.WriteString(styleDiffAdd.Render(seg))
				out.WriteString("\n")
			}
		}
	}
	return out.String()
}

func (c diffCounts) String() string {
	return fmt.Sprintf("+%d -%d ~%d", c.Added, c.Removed, c.Changed)
}

func renderDiffLinesGitLike(lines []diffLine, maxLines int) string {
	if maxLines <= 0 {
		maxLines = 5000
	}
	sort.Slice(lines, func(i, j int) bool {
		if lines[i].Path != lines[j].Path {
			return lines[i].Path < lines[j].Path
		}
		// Stable: removals before changes before additions at same path.
		order := func(k string) int {
			switch k {
			case "-":
				return 0
			case "~":
				return 1
			case "+":
				return 2
			default:
				return 3
			}
		}
		if order(lines[i].Kind) != order(lines[j].Kind) {
			return order(lines[i].Kind) < order(lines[j].Kind)
		}
		return lines[i].Text < lines[j].Text
	})
	out := strings.Builder{}
	// git-ish headers. Important: do NOT include newlines inside Render(),
	// otherwise the terminal reset sequence may land on the next line and some
	// renderers/copy-paste flows produce odd indentation.
	out.WriteString(styleDiffHdr.Render("diff --git a/values b/values"))
	out.WriteString("\n")
	out.WriteString(styleDiffHdr.Render("--- a/values"))
	out.WriteString("\n")
	out.WriteString(styleDiffHdr.Render("+++ b/values"))
	out.WriteString("\n")
	limit := len(lines)
	truncated := false
	if maxLines > 0 && limit > maxLines {
		limit = maxLines
		truncated = true
	}
	for i := 0; i < limit; i++ {
		// Render roughly like a unified diff line.
		prefix := lines[i].Kind
		line := prefix + " " + lines[i].Path + ": " + lines[i].Text
		switch prefix {
		case "+":
			out.WriteString(styleDiffAdd.Render(line))
		case "-":
			out.WriteString(styleDiffDel.Render(line))
		default:
			out.WriteString(line)
		}
		out.WriteString("\n")
	}
	if truncated {
		out.WriteString(styleDiffHdr.Render("… truncated …"))
		out.WriteString("\n")
	}
	return out.String()
}

func renderDiffLinesGitLikeWrapped(lines []diffLine, maxLines int, width int) string {
	// Render git-like diff then optionally word-wrap it to width.
	text := renderDiffLinesGitLike(lines, maxLines)
	if width <= 0 {
		return text
	}
	// Wrap each line independently to preserve ANSI coloring boundaries.
	linesIn := strings.Split(text, "\n")
	out := strings.Builder{}
	for i, ln := range linesIn {
		if i == len(linesIn)-1 && ln == "" {
			break
		}
		for _, seg := range wrapLine(ln, width, true) {
			out.WriteString(seg)
			out.WriteString("\n")
		}
	}
	return out.String()
}

func wrapTextPlain(text string, width int) string {
	if width <= 0 {
		return text
	}
	linesIn := strings.Split(text, "\n")
	out := strings.Builder{}
	for i, ln := range linesIn {
		if i == len(linesIn)-1 && ln == "" {
			break
		}
		for _, seg := range wrapLine(ln, width, true) {
			out.WriteString(seg)
			out.WriteString("\n")
		}
	}
	return out.String()
}

func diffRowsFromLines(lines []diffLine, maxRows int) (rows []diffRow) {
	// Consolidate by path. We expect at most one '-' and one '+' per path.
	// Ordering: stable by path.
	if maxRows <= 0 {
		maxRows = 5000
	}
	byPath := map[string]*diffRow{}
	paths := make([]string, 0, len(lines))
	for i := range lines {
		p := lines[i].Path
		r := byPath[p]
		if r == nil {
			r = &diffRow{Path: p}
			byPath[p] = r
			paths = append(paths, p)
		}
		switch lines[i].Kind {
		case "-":
			r.Old = lines[i].Text
		case "+":
			r.New = lines[i].Text
		}
	}
	sort.Strings(paths)
	paths = uniqueStringsSorted(paths)
	if len(paths) > maxRows {
		paths = paths[:maxRows]
	}
	rows = make([]diffRow, 0, len(paths))
	for _, p := range paths {
		r := byPath[p]
		if r == nil {
			continue
		}
		// Classify kind.
		kind := "chg"
		if strings.TrimSpace(r.Old) == "" && strings.TrimSpace(r.New) != "" {
			kind = "add"
		} else if strings.TrimSpace(r.Old) != "" && strings.TrimSpace(r.New) == "" {
			kind = "del"
		}
		rows = append(rows, diffRow{Path: r.Path, Old: r.Old, New: r.New, Kind: kind})
	}
	return rows
}

func renderSideBySide(rows []diffRow, totalWidth int, wrap bool) string {
	// Render a single text block where each visual line contains both columns.
	//
	// Important: wrap/truncation must be applied *before* styling, otherwise ANSI
	// sequences break width calculations.
	if totalWidth <= 0 {
		totalWidth = 120
	}
	sep := " │ "
	colW := (totalWidth - len(sep)) / 2
	if colW < 20 {
		colW = 20
	}

	padRight := func(s string, w int) string {
		r := []rune(s)
		if len(r) >= w {
			return s
		}
		return s + strings.Repeat(" ", w-len(r))
	}
	trimTo := func(s string, w int) string {
		r := []rune(s)
		if len(r) <= w {
			return s
		}
		if w <= 1 {
			return string(r[:w])
		}
		return string(r[:w-1]) + "…"
	}

	cellLines := func(s string) []string {
		// Backward-compatible path: no intraline styling.
		s = strings.TrimRight(s, " ")
		if s == "" {
			return []string{""}
		}
		if wrap {
			return wrapLine(s, colW, true)
		}
		return []string{trimTo(s, colW)}
	}

	out := strings.Builder{}
	// Header (keep it simple; no wrap).
	leftHdr := padRight(trimTo("old", colW), colW)
	rightHdr := padRight(trimTo("new", colW), colW)
	out.WriteString(styleDiffHdr.Render(leftHdr))
	out.WriteString(styleDiffHdr.Render(sep))
	out.WriteString(styleDiffHdr.Render(rightHdr))
	out.WriteString("\n")

	for _, r := range rows {
		isChanged := strings.TrimSpace(r.Old) != "" && strings.TrimSpace(r.New) != ""
		leftPlain := ""
		rightPlain := ""
		if strings.TrimSpace(r.Old) != "" {
			leftPlain = "- " + r.Path + ": " + r.Old
		}
		if strings.TrimSpace(r.New) != "" {
			rightPlain = "+ " + r.Path + ": " + r.New
		}

		var leftLines [][]styledSpan
		var rightLines [][]styledSpan
		if isChanged {
			// Left (old) with intraline highlights.
			lpfx, lval, lok := splitValueForIntraline(leftPlain)
			rpfx, rval, rok := splitValueForIntraline(rightPlain)
			if lok && rok {
				lsp := intralineSpans(lpfx, lval, rval, styleDiffDel, styleDiffDelIntra)
				rsp := func() []styledSpan {
					p := commonPrefixLenRunes(lval, rval)
					s := commonSuffixLenRunes(lval, rval, p)
					rn := []rune(rval)
					commonPre := string(rn[:p])
					commonSuf := ""
					if s > 0 {
						commonSuf = string(rn[len(rn)-s:])
					}
					mid := string(rn[p : len(rn)-s])
					sp := []styledSpan{{Text: rpfx, Style: styleDiffAdd}, {Text: commonPre, Style: styleDiffAdd}}
					if strings.TrimSpace(mid) != "" {
						sp = append(sp, styledSpan{Text: mid, Style: styleDiffAddIntra})
					}
					sp = append(sp, styledSpan{Text: commonSuf, Style: styleDiffAdd})
					return sp
				}()
				wrapW := 0
				if wrap {
					wrapW = colW
				}
				leftLines = wrapSpans(lsp, wrapW)
				rightLines = wrapSpans(rsp, wrapW)
			} else {
				// Fallback to plain if we can't split value.
				leftLines = wrapSpans([]styledSpan{{Text: trimTo(leftPlain, colW), Style: styleDiffDel}}, 0)
				rightLines = wrapSpans([]styledSpan{{Text: trimTo(rightPlain, colW), Style: styleDiffAdd}}, 0)
			}
		} else {
			// Added/removed: plain whole-line styling.
			if strings.TrimSpace(leftPlain) != "" {
				txt := leftPlain
				if !wrap {
					txt = trimTo(txt, colW)
				}
				wrapW := 0
				if wrap {
					wrapW = colW
				}
				leftLines = wrapSpans([]styledSpan{{Text: txt, Style: styleDiffDel}}, wrapW)
			} else {
				leftLines = [][]styledSpan{{}}
			}
			if strings.TrimSpace(rightPlain) != "" {
				txt := rightPlain
				if !wrap {
					txt = trimTo(txt, colW)
				}
				wrapW := 0
				if wrap {
					wrapW = colW
				}
				rightLines = wrapSpans([]styledSpan{{Text: txt, Style: styleDiffAdd}}, wrapW)
			} else {
				rightLines = [][]styledSpan{{}}
			}
		}

		// Also keep old plain path for header-only rows.
		_ = cellLines

		height := len(leftLines)
		if len(rightLines) > height {
			height = len(rightLines)
		}
		for i := 0; i < height; i++ {
			lsp := []styledSpan{}
			rsp := []styledSpan{}
			if i < len(leftLines) {
				lsp = leftLines[i]
			}
			if i < len(rightLines) {
				rsp = rightLines[i]
			}
			lRendered := renderSpans(lsp)
			rRendered := renderSpans(rsp)
			// Ensure stable column alignment by padding based on rune width of the
			// *plain* content lengths. We approximate by stripping styles: use the
			// span text lengths.
			lPlainW := 0
			for _, sp := range lsp {
				lPlainW += spanTextLen(sp)
			}
			rPlainW := 0
			for _, sp := range rsp {
				rPlainW += spanTextLen(sp)
			}
			if lPlainW < colW {
				lRendered += strings.Repeat(" ", colW-lPlainW)
			}
			if rPlainW < colW {
				rRendered += strings.Repeat(" ", colW-rPlainW)
			}
			out.WriteString(lRendered)
			out.WriteString(styleDiffHdr.Render(sep))
			out.WriteString(rRendered)
			out.WriteString("\n")
		}
	}
	return out.String()
}

func wrapLine(line string, width int, wrap bool) []string {
	if !wrap || width <= 0 {
		return []string{line}
	}
	r := []rune(line)
	if len(r) <= width {
		return []string{line}
	}
	parts := []string{}
	for len(r) > 0 {
		if len(r) <= width {
			parts = append(parts, string(r))
			break
		}
		cut := width
		// Prefer breaking on the last space within the segment.
		lastSpace := -1
		for i := 0; i < cut; i++ {
			if r[i] == ' ' {
				lastSpace = i
			}
		}
		if lastSpace > 0 {
			cut = lastSpace
		}
		seg := strings.TrimRight(string(r[:cut]), " ")
		parts = append(parts, seg)
		// Skip leading spaces on next line.
		r = r[cut:]
		r = []rune(strings.TrimLeft(string(r), " "))
	}
	return parts
}

func DiffJSONSchema(oldRaw, newRaw string) (rendered string, counts diffCounts, ok bool, err error) {
	oldRaw = strings.TrimSpace(oldRaw)
	newRaw = strings.TrimSpace(newRaw)
	if oldRaw == "" || newRaw == "" {
		return "", diffCounts{}, false, nil
	}

	parse := func(s string) (any, error) {
		var v any
		dec := json.NewDecoder(strings.NewReader(s))
		dec.UseNumber()
		if err := dec.Decode(&v); err != nil {
			return nil, err
		}
		return v, nil
	}
	oldV, err := parse(oldRaw)
	if err != nil {
		return "", diffCounts{}, false, fmt.Errorf("parse old schema: %w", err)
	}
	newV, err := parse(newRaw)
	if err != nil {
		return "", diffCounts{}, false, fmt.Errorf("parse new schema: %w", err)
	}

	lines := []diffLine{}
	var walk func(path string, a, b any)
	walk = func(path string, a, b any) {
		if a == nil && b == nil {
			return
		}
		if a == nil {
			// Expand objects/arrays so large containers don't become unreadable one-liners.
			switch bb := b.(type) {
			case map[string]any:
				// Optional container marker.
				counts.Added++
				lines = append(lines, diffLine{Path: path, Kind: "+", Text: "{…}"})
				keys := make([]string, 0, len(bb))
				for k := range bb {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					walk(joinJSONPath(path, k), nil, bb[k])
				}
				return
			case []any:
				counts.Added++
				lines = append(lines, diffLine{Path: path, Kind: "+", Text: fmt.Sprintf("[%d]", len(bb))})
				for i := range bb {
					walk(fmt.Sprintf("%s[%d]", path, i), nil, bb[i])
				}
				return
			default:
				counts.Added++
				lines = append(lines, diffLine{Path: path, Kind: "+", Text: renderJSONValueShort(b)})
				return
			}
		}
		if b == nil {
			// Expand objects/arrays for removals too.
			switch aa := a.(type) {
			case map[string]any:
				counts.Removed++
				lines = append(lines, diffLine{Path: path, Kind: "-", Text: "{…}"})
				keys := make([]string, 0, len(aa))
				for k := range aa {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					walk(joinJSONPath(path, k), aa[k], nil)
				}
				return
			case []any:
				counts.Removed++
				lines = append(lines, diffLine{Path: path, Kind: "-", Text: fmt.Sprintf("[%d]", len(aa))})
				for i := range aa {
					walk(fmt.Sprintf("%s[%d]", path, i), aa[i], nil)
				}
				return
			default:
				counts.Removed++
				lines = append(lines, diffLine{Path: path, Kind: "-", Text: renderJSONValueShort(a)})
				return
			}
		}
		switch aa := a.(type) {
		case map[string]any:
			bb, ok := b.(map[string]any)
			if !ok {
				counts.Changed++
				lines = append(lines,
					diffLine{Path: path, Kind: "-", Text: renderJSONValueShort(a)},
					diffLine{Path: path, Kind: "+", Text: renderJSONValueShort(b)},
				)
				return
			}
			keys := make([]string, 0, len(aa)+len(bb))
			seen := map[string]struct{}{}
			for k := range aa {
				seen[k] = struct{}{}
				keys = append(keys, k)
			}
			for k := range bb {
				if _, ok := seen[k]; !ok {
					keys = append(keys, k)
				}
			}
			sort.Strings(keys)
			for _, k := range keys {
				np := joinJSONPath(path, k)
				walk(np, aa[k], bb[k])
			}
			return
		case []any:
			bb, ok := b.([]any)
			if !ok {
				counts.Changed++
				lines = append(lines,
					diffLine{Path: path, Kind: "-", Text: renderJSONValueShort(a)},
					diffLine{Path: path, Kind: "+", Text: renderJSONValueShort(b)},
				)
				return
			}
			maxN := len(aa)
			if len(bb) > maxN {
				maxN = len(bb)
			}
			for i := 0; i < maxN; i++ {
				var av any
				if i < len(aa) {
					av = aa[i]
				}
				var bv any
				if i < len(bb) {
					bv = bb[i]
				}
				np := fmt.Sprintf("%s[%d]", path, i)
				walk(np, av, bv)
			}
			return
		default:
			if !jsonScalarEqual(a, b) {
				counts.Changed++
				lines = append(lines,
					diffLine{Path: path, Kind: "-", Text: renderJSONValueShort(a)},
					diffLine{Path: path, Kind: "+", Text: renderJSONValueShort(b)},
				)
			}
			return
		}
	}

	walk("#", oldV, newV)
	if len(lines) == 0 {
		return "(no schema changes detected)\n", counts, true, nil
	}
	// Note: the modal has its own viewport; truncation here makes content
	// unreachable. Keep this unlimited.
	return renderDiffLinesGitLike(lines, -1), counts, true, nil
}

func DiffJSONSchemaRows(oldRaw, newRaw string) (rows []diffRow, counts diffCounts, ok bool, err error) {
	oldRaw = strings.TrimSpace(oldRaw)
	newRaw = strings.TrimSpace(newRaw)
	if oldRaw == "" || newRaw == "" {
		return nil, diffCounts{}, false, nil
	}
	parse := func(s string) (any, error) {
		var v any
		dec := json.NewDecoder(strings.NewReader(s))
		dec.UseNumber()
		if err := dec.Decode(&v); err != nil {
			return nil, err
		}
		return v, nil
	}
	oldV, err := parse(oldRaw)
	if err != nil {
		return nil, diffCounts{}, false, fmt.Errorf("parse old schema: %w", err)
	}
	newV, err := parse(newRaw)
	if err != nil {
		return nil, diffCounts{}, false, fmt.Errorf("parse new schema: %w", err)
	}
	lines := []diffLine{}
	var walk func(path string, a, b any)
	walk = func(path string, a, b any) {
		if a == nil && b == nil {
			return
		}
		if a == nil {
			switch bb := b.(type) {
			case map[string]any:
				counts.Added++
				lines = append(lines, diffLine{Path: path, Kind: "+", Text: "{…}"})
				keys := make([]string, 0, len(bb))
				for k := range bb {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					walk(joinJSONPath(path, k), nil, bb[k])
				}
				return
			case []any:
				counts.Added++
				lines = append(lines, diffLine{Path: path, Kind: "+", Text: fmt.Sprintf("[%d]", len(bb))})
				for i := range bb {
					walk(fmt.Sprintf("%s[%d]", path, i), nil, bb[i])
				}
				return
			default:
				counts.Added++
				lines = append(lines, diffLine{Path: path, Kind: "+", Text: renderJSONValueShort(b)})
				return
			}
		}
		if b == nil {
			switch aa := a.(type) {
			case map[string]any:
				counts.Removed++
				lines = append(lines, diffLine{Path: path, Kind: "-", Text: "{…}"})
				keys := make([]string, 0, len(aa))
				for k := range aa {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					walk(joinJSONPath(path, k), aa[k], nil)
				}
				return
			case []any:
				counts.Removed++
				lines = append(lines, diffLine{Path: path, Kind: "-", Text: fmt.Sprintf("[%d]", len(aa))})
				for i := range aa {
					walk(fmt.Sprintf("%s[%d]", path, i), aa[i], nil)
				}
				return
			default:
				counts.Removed++
				lines = append(lines, diffLine{Path: path, Kind: "-", Text: renderJSONValueShort(a)})
				return
			}
		}
		switch aa := a.(type) {
		case map[string]any:
			bb, ok := b.(map[string]any)
			if !ok {
				counts.Changed++
				lines = append(lines,
					diffLine{Path: path, Kind: "-", Text: renderJSONValueShort(a)},
					diffLine{Path: path, Kind: "+", Text: renderJSONValueShort(b)},
				)
				return
			}
			keys := make([]string, 0, len(aa)+len(bb))
			seen := map[string]struct{}{}
			for k := range aa {
				seen[k] = struct{}{}
				keys = append(keys, k)
			}
			for k := range bb {
				if _, ok := seen[k]; !ok {
					keys = append(keys, k)
				}
			}
			sort.Strings(keys)
			for _, k := range keys {
				np := joinJSONPath(path, k)
				walk(np, aa[k], bb[k])
			}
			return
		case []any:
			bb, ok := b.([]any)
			if !ok {
				counts.Changed++
				lines = append(lines,
					diffLine{Path: path, Kind: "-", Text: renderJSONValueShort(a)},
					diffLine{Path: path, Kind: "+", Text: renderJSONValueShort(b)},
				)
				return
			}
			maxN := len(aa)
			if len(bb) > maxN {
				maxN = len(bb)
			}
			for i := 0; i < maxN; i++ {
				var av any
				if i < len(aa) {
					av = aa[i]
				}
				var bv any
				if i < len(bb) {
					bv = bb[i]
				}
				np := fmt.Sprintf("%s[%d]", path, i)
				walk(np, av, bv)
			}
			return
		default:
			if !jsonScalarEqual(a, b) {
				counts.Changed++
				lines = append(lines,
					diffLine{Path: path, Kind: "-", Text: renderJSONValueShort(a)},
					diffLine{Path: path, Kind: "+", Text: renderJSONValueShort(b)},
				)
			}
			return
		}
	}
	walk("#", oldV, newV)
	if len(lines) == 0 {
		return []diffRow{}, counts, true, nil
	}
	rows = diffRowsFromLines(lines, 50000)
	return rows, counts, true, nil
}

func joinJSONPath(base, key string) string {
	// Keep it readable and stable. This is not strict JSON Pointer.
	if base == "" {
		return key
	}
	return base + "/" + key
}

func jsonScalarEqual(a, b any) bool {
	// json.Decoder with UseNumber yields json.Number for numbers.
	sA, okA := a.(json.Number)
	sB, okB := b.(json.Number)
	if okA && okB {
		return sA.String() == sB.String()
	}
	// Normalize some common scalar types.
	return fmt.Sprintf("%T:%v", a, a) == fmt.Sprintf("%T:%v", b, b)
}

func renderJSONValueShort(v any) string {
	if v == nil {
		return "null"
	}
	switch vv := v.(type) {
	case string:
		return strconv.Quote(vv)
	case json.Number:
		return vv.String()
	case bool:
		if vv {
			return "true"
		}
		return "false"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	s := string(b)
	// Prevent enormous blobs in the modal.
	const max = 240
	if len(s) > max {
		s = s[:max] + "…"
	}
	return s
}

func DiffYAMLValues(oldRaw, newRaw string) (rendered string, counts diffCounts, ok bool, err error) {
	oldRaw = strings.TrimSpace(oldRaw)
	newRaw = strings.TrimSpace(newRaw)
	if oldRaw == "" || newRaw == "" {
		return "", diffCounts{}, false, nil
	}

	parse := func(s string) (any, error) {
		var v any
		if err := yaml.Unmarshal([]byte(s), &v); err != nil {
			return nil, err
		}
		return v, nil
	}
	oldV, err := parse(oldRaw)
	if err != nil {
		return "", diffCounts{}, false, fmt.Errorf("parse old values: %w", err)
	}
	newV, err := parse(newRaw)
	if err != nil {
		return "", diffCounts{}, false, fmt.Errorf("parse new values: %w", err)
	}

	oldM := map[string]string{}
	newM := map[string]string{}
	flattenYAMLValues(oldM, "$", oldV)
	flattenYAMLValues(newM, "$", newV)

	paths := make([]string, 0, len(oldM)+len(newM))
	seen := map[string]struct{}{}
	for p := range oldM {
		seen[p] = struct{}{}
		paths = append(paths, p)
	}
	for p := range newM {
		if _, ok := seen[p]; !ok {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	paths = uniqueStringsSorted(paths)

	lines := []diffLine{}
	for _, p := range paths {
		a, okA := oldM[p]
		b, okB := newM[p]
		if !okA && okB {
			counts.Added++
			lines = append(lines, diffLine{Path: p, Kind: "+", Text: b})
			continue
		}
		if okA && !okB {
			counts.Removed++
			lines = append(lines, diffLine{Path: p, Kind: "-", Text: a})
			continue
		}
		if okA && okB && a != b {
			counts.Changed++
			lines = append(lines,
				diffLine{Path: p, Kind: "-", Text: a},
				diffLine{Path: p, Kind: "+", Text: b},
			)
		}
	}
	if len(lines) == 0 {
		return "(no values changes detected)\n", counts, true, nil
	}
	// Note: the modal has its own viewport; truncation here makes content
	// unreachable. Keep this unlimited.
	return renderDiffLinesGitLike(lines, -1), counts, true, nil
}

func DiffYAMLValuesRows(oldRaw, newRaw string) (rows []diffRow, counts diffCounts, ok bool, err error) {
	oldRaw = strings.TrimSpace(oldRaw)
	newRaw = strings.TrimSpace(newRaw)
	if oldRaw == "" || newRaw == "" {
		return nil, diffCounts{}, false, nil
	}
	parse := func(s string) (any, error) {
		var v any
		if err := yaml.Unmarshal([]byte(s), &v); err != nil {
			return nil, err
		}
		return v, nil
	}
	oldV, err := parse(oldRaw)
	if err != nil {
		return nil, diffCounts{}, false, fmt.Errorf("parse old values: %w", err)
	}
	newV, err := parse(newRaw)
	if err != nil {
		return nil, diffCounts{}, false, fmt.Errorf("parse new values: %w", err)
	}
	oldM := map[string]string{}
	newM := map[string]string{}
	flattenYAMLValues(oldM, "$", oldV)
	flattenYAMLValues(newM, "$", newV)

	paths := make([]string, 0, len(oldM)+len(newM))
	seen := map[string]struct{}{}
	for p := range oldM {
		seen[p] = struct{}{}
		paths = append(paths, p)
	}
	for p := range newM {
		if _, ok := seen[p]; !ok {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	paths = uniqueStringsSorted(paths)

	lines := []diffLine{}
	for _, p := range paths {
		a, okA := oldM[p]
		b, okB := newM[p]
		if !okA && okB {
			counts.Added++
			lines = append(lines, diffLine{Path: p, Kind: "+", Text: b})
			continue
		}
		if okA && !okB {
			counts.Removed++
			lines = append(lines, diffLine{Path: p, Kind: "-", Text: a})
			continue
		}
		if okA && okB && a != b {
			counts.Changed++
			lines = append(lines,
				diffLine{Path: p, Kind: "-", Text: a},
				diffLine{Path: p, Kind: "+", Text: b},
			)
		}
	}
	if len(lines) == 0 {
		return []diffRow{}, counts, true, nil
	}
	rows = diffRowsFromLines(lines, 5000)
	return rows, counts, true, nil
}

func uniqueStringsSorted(in []string) []string {
	if len(in) < 2 {
		return in
	}
	out := make([]string, 0, len(in))
	prev := ""
	for i, s := range in {
		if i == 0 || s != prev {
			out = append(out, s)
		}
		prev = s
	}
	return out
}

func flattenYAMLValues(out map[string]string, path string, v any) {
	// Record empty containers as leaf values so they can be diffed.
	switch vv := v.(type) {
	case map[string]any:
		if len(vv) == 0 {
			out[path] = "{}"
			return
		}
		keys := make([]string, 0, len(vv))
		for k := range vv {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			flattenYAMLValues(out, path+"."+k, vv[k])
		}
		return
	case map[any]any:
		if len(vv) == 0 {
			out[path] = "{}"
			return
		}
		keys := make([]string, 0, len(vv))
		kv := map[string]any{}
		for k, val := range vv {
			ks := fmt.Sprintf("%v", k)
			keys = append(keys, ks)
			kv[ks] = val
		}
		sort.Strings(keys)
		for _, k := range keys {
			flattenYAMLValues(out, path+"."+k, kv[k])
		}
		return
	case []any:
		if len(vv) == 0 {
			out[path] = "[]"
			return
		}
		for i := range vv {
			flattenYAMLValues(out, fmt.Sprintf("%s[%d]", path, i), vv[i])
		}
		return
	default:
		out[path] = renderYAMLScalarShort(v)
		return
	}
}

func renderYAMLScalarShort(v any) string {
	if v == nil {
		return "null"
	}
	switch vv := v.(type) {
	case string:
		return strconv.Quote(vv)
	case bool:
		if vv {
			return "true"
		}
		return "false"
	case int:
		return strconv.Itoa(vv)
	case int64:
		return strconv.FormatInt(vv, 10)
	case float64:
		return strconv.FormatFloat(vv, 'f', -1, 64)
	default:
		s := fmt.Sprintf("%v", v)
		const max = 240
		if len(s) > max {
			s = s[:max] + "…"
		}
		return s
	}
}
