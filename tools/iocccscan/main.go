package main

import (
	"archive/tar"
	"compress/bzip2"
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const yearsURL = "https://www.ioccc.org/years.html"

type entryManifestFile struct {
	FilePath  string `json:"file_path"`
	DisplayAs string `json:"display_as"`
	EntryText string `json:"entry_text"`
}

type entryManifest struct {
	Year     int                 `json:"year"`
	Dir      string              `json:"dir"`
	EntryID  string              `json:"entry_id"`
	Title    string              `json:"title"`
	Award    string              `json:"award"`
	Manifest []entryManifestFile `json:"manifest"`
}

type entryCase struct {
	Year      int
	EntryID   string
	Dir       string
	Title     string
	Award     string
	SourceRel string
	SourceAbs string
	Defines   []string
	Includes  []string
}

type stageOutcome struct {
	Name     string
	OK       bool
	Duration time.Duration
	ErrMsg   string
}

type resultRow struct {
	Year         int
	EntryID      string
	Dir          string
	Title        string
	Award        string
	SourceRel    string
	Category     string
	PreprocessMS int64
	ParseMS      int64
	CompileMS    int64
	ErrorSummary string
}

type summaryCounts struct {
	total          int
	ok             int
	preprocessFail int
	parseFail      int
	compileFail    int
	skipped        int
}

func main() {
	var (
		yearsFlag              string
		limit                  int
		cacheDir               string
		extractDir             string
		reportPath             string
		compiler               string
		target                 string
		timeout                time.Duration
		keepExtract            bool
		hostCC                 string
		discoverSystemIncludes bool
	)

	flag.StringVar(&yearsFlag, "years", "", "comma-separated IOCCC years to scan (default: discover all year tarballs from the IOCCC site)")
	flag.IntVar(&limit, "limit", 0, "maximum number of entries to scan after discovery")
	flag.StringVar(&cacheDir, "cache-dir", filepath.Join("build", "ioccc-cache"), "directory for downloaded IOCCC tarballs")
	flag.StringVar(&extractDir, "extract-dir", filepath.Join("build", "ioccc-work"), "directory for extracted IOCCC tarballs")
	flag.StringVar(&reportPath, "report", filepath.Join("build", "ioccc-report.csv"), "CSV report path")
	flag.StringVar(&compiler, "compiler", filepath.Join("build", "rtg"), "path to RTG compiler binary")
	flag.StringVar(&target, "target", "c/64", "target passed to RTG for compile-to-IR stage")
	flag.DurationVar(&timeout, "timeout", 20*time.Second, "per-stage timeout")
	flag.BoolVar(&keepExtract, "keep-extract", true, "keep extracted tarball contents between runs")
	flag.StringVar(&hostCC, "host-cc", "", "host C compiler used to discover system include paths (default: $CC or cc)")
	flag.BoolVar(&discoverSystemIncludes, "discover-system-includes", true, "discover host system include paths and pass them via -isystem")
	flag.Parse()

	if timeout <= 0 {
		fmt.Fprintf(os.Stderr, "iocccscan: timeout must be > 0\n")
		os.Exit(1)
	}

	years, err := resolveYears(yearsFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "iocccscan: %v\n", err)
		os.Exit(1)
	}
	if len(years) == 0 {
		fmt.Fprintf(os.Stderr, "iocccscan: no years selected\n")
		os.Exit(1)
	}

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "iocccscan: create cache dir: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "iocccscan: create extract dir: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Dir(reportPath), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "iocccscan: create report dir: %v\n", err)
		os.Exit(1)
	}

	var cases []entryCase
	for _, year := range years {
		fmt.Printf("year %d: preparing tarball\n", year)
		yearDir, err := prepareYear(year, cacheDir, extractDir, keepExtract)
		if err != nil {
			fmt.Fprintf(os.Stderr, "iocccscan: year %d: %v\n", year, err)
			os.Exit(1)
		}
		yearCases, err := discoverCases(yearDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "iocccscan: year %d: %v\n", year, err)
			os.Exit(1)
		}
		fmt.Printf("year %d: discovered %d entry sources\n", year, len(yearCases))
		cases = append(cases, yearCases...)
	}

	sort.Slice(cases, func(i, j int) bool {
		if cases[i].Year != cases[j].Year {
			return cases[i].Year > cases[j].Year
		}
		return cases[i].EntryID < cases[j].EntryID
	})
	if limit > 0 && limit < len(cases) {
		cases = cases[:limit]
	}
	if len(cases) == 0 {
		fmt.Fprintf(os.Stderr, "iocccscan: no entry cases discovered\n")
		os.Exit(1)
	}

	compilerPath, err := filepath.Abs(compiler)
	if err != nil {
		fmt.Fprintf(os.Stderr, "iocccscan: resolve compiler path: %v\n", err)
		os.Exit(1)
	}
	if _, err := os.Stat(compilerPath); err != nil {
		fmt.Fprintf(os.Stderr, "iocccscan: compiler %q not found: %v\n", compilerPath, err)
		os.Exit(1)
	}

	var systemIncludes []string
	if discoverSystemIncludes {
		if hostCC == "" {
			hostCC = os.Getenv("CC")
			if hostCC == "" {
				hostCC = "cc"
			}
		}
		systemIncludes, err = discoverHostSystemIncludes(hostCC)
		if err != nil {
			fmt.Fprintf(os.Stderr, "iocccscan: discover system includes with %q: %v\n", hostCC, err)
			os.Exit(1)
		}
		fmt.Printf("system includes: %d directories from %s\n", len(systemIncludes), hostCC)
	}

	rows := make([]resultRow, 0, len(cases))
	var summary summaryCounts
	summary.total = len(cases)
	for i, tc := range cases {
		fmt.Printf("[%d/%d] %s (%s)\n", i+1, len(cases), tc.EntryID, tc.SourceRel)
		row := runCase(tc, compilerPath, target, timeout, systemIncludes)
		rows = append(rows, row)
		switch row.Category {
		case "ok":
			summary.ok++
		case "preprocess_fail":
			summary.preprocessFail++
		case "parse_fail":
			summary.parseFail++
		case "compile_fail":
			summary.compileFail++
		default:
			summary.skipped++
		}
	}

	if err := writeCSV(reportPath, rows); err != nil {
		fmt.Fprintf(os.Stderr, "iocccscan: write report: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nreport: %s\n", reportPath)
	fmt.Printf("total=%d ok=%d preprocess_fail=%d parse_fail=%d compile_fail=%d skipped=%d\n",
		summary.total, summary.ok, summary.preprocessFail, summary.parseFail, summary.compileFail, summary.skipped)
}

func resolveYears(yearsFlag string) ([]int, error) {
	if strings.TrimSpace(yearsFlag) != "" {
		parts := strings.Split(yearsFlag, ",")
		seen := make(map[int]bool)
		var years []int
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			var year int
			if _, err := fmt.Sscanf(part, "%d", &year); err != nil {
				return nil, fmt.Errorf("invalid year %q", part)
			}
			if !seen[year] {
				seen[year] = true
				years = append(years, year)
			}
		}
		sort.Slice(years, func(i, j int) bool { return years[i] > years[j] })
		return years, nil
	}

	req, err := http.NewRequest(http.MethodGet, yearsURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "rtg-iocccscan/1")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch years page: %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	re := regexp.MustCompile(`href="([0-9]{4})/[0-9]{4}\.tar\.bz2"`)
	matches := re.FindAllStringSubmatch(string(body), -1)
	seen := make(map[int]bool)
	var years []int
	for _, m := range matches {
		var year int
		if _, err := fmt.Sscanf(m[1], "%d", &year); err != nil {
			continue
		}
		if !seen[year] {
			seen[year] = true
			years = append(years, year)
		}
	}
	sort.Slice(years, func(i, j int) bool { return years[i] > years[j] })
	return years, nil
}

func prepareYear(year int, cacheDir string, extractDir string, keepExtract bool) (string, error) {
	tarName := fmt.Sprintf("%d.tar.bz2", year)
	tarPath := filepath.Join(cacheDir, tarName)
	if _, err := os.Stat(tarPath); err != nil {
		if !os.IsNotExist(err) {
			return "", err
		}
		url := fmt.Sprintf("https://www.ioccc.org/%d/%s", year, tarName)
		if err := downloadFile(url, tarPath); err != nil {
			return "", err
		}
	}

	yearDir := filepath.Join(extractDir, fmt.Sprintf("%d", year))
	marker := filepath.Join(yearDir, ".extract-ok")
	if keepExtract {
		if _, err := os.Stat(marker); err == nil {
			return yearDir, nil
		}
	}
	if err := os.RemoveAll(yearDir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(yearDir, 0o755); err != nil {
		return "", err
	}
	if err := extractTarBz2(tarPath, yearDir); err != nil {
		return "", err
	}
	if err := os.WriteFile(marker, []byte("ok\n"), 0o644); err != nil {
		return "", err
	}
	return yearDir, nil
}

func downloadFile(url string, dst string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "rtg-iocccscan/1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: %s", url, resp.Status)
	}
	tmp := dst + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

func extractTarBz2(src string, dst string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	tr := tar.NewReader(bzip2.NewReader(f))
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(hdr.Name)
		if name == "." {
			continue
		}
		target := filepath.Join(dst, name)
		if !strings.HasPrefix(target, filepath.Clean(dst)+string(os.PathSeparator)) && filepath.Clean(target) != filepath.Clean(dst) {
			return fmt.Errorf("tar entry escapes destination: %q", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
			mode := hdr.FileInfo().Mode() & 0o777
			if mode == 0 {
				mode = 0o644
			}
			if err := os.Chmod(target, mode); err != nil {
				return err
			}
		}
	}
}

func discoverCases(yearDir string) ([]entryCase, error) {
	var entries []entryCase
	err := filepath.WalkDir(yearDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != ".entry.json" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var manifest entryManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			return fmt.Errorf("%s: parse manifest: %w", path, err)
		}
		srcRel := canonicalSource(manifest)
		if srcRel == "" {
			return nil
		}
		entryDir := filepath.Dir(path)
		srcAbs := filepath.Join(entryDir, srcRel)
		if _, err := os.Stat(srcAbs); err != nil {
			return fmt.Errorf("%s: source %q missing: %w", path, srcRel, err)
		}
		defines, includes, err := discoverEntryBuildFlags(entryDir)
		if err != nil {
			return fmt.Errorf("%s: parse build flags: %w", path, err)
		}
		entries = append(entries, entryCase{
			Year:      manifest.Year,
			EntryID:   manifest.EntryID,
			Dir:       manifest.Dir,
			Title:     manifest.Title,
			Award:     manifest.Award,
			SourceRel: srcRel,
			SourceAbs: srcAbs,
			Defines:   defines,
			Includes:  includes,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

func canonicalSource(m entryManifest) string {
	for _, item := range m.Manifest {
		if item.EntryText == "entry source code" && strings.HasSuffix(item.FilePath, ".c") {
			return item.FilePath
		}
	}
	return ""
}

func runCase(tc entryCase, compilerPath string, target string, timeout time.Duration, systemIncludes []string) resultRow {
	baseDir := os.TempDir()
	prefix := fmt.Sprintf("iocccscan_%s", strings.ReplaceAll(tc.EntryID, "/", "_"))
	preOut := filepath.Join(baseDir, prefix+".tokens")
	parseOut := filepath.Join(baseDir, prefix+".ast")
	irOut := filepath.Join(baseDir, prefix+".ir")
	srcPath := tc.SourceAbs
	defer os.Remove(preOut)
	defer os.Remove(parseOut)
	defer os.Remove(irOut)
	if len(tc.Includes) > 0 {
		wrapperPath := filepath.Join(baseDir, prefix+".wrapper.c")
		if err := writeWrapperSource(wrapperPath, tc.Includes, tc.SourceAbs); err != nil {
			return resultRow{
				Year:         tc.Year,
				EntryID:      tc.EntryID,
				Dir:          tc.Dir,
				Title:        tc.Title,
				Award:        tc.Award,
				SourceRel:    tc.SourceRel,
				Category:     "preprocess_fail",
				ErrorSummary: fmt.Sprintf("wrapper generation error: %v", err),
			}
		}
		defer os.Remove(wrapperPath)
		srcPath = wrapperPath
	}

	preArgs := append(cArgs(systemIncludes, tc.Defines), "-x", "c99", "-E", "-o", preOut, srcPath)
	pre := runStage(timeout, compilerPath, preArgs...)
	if !pre.OK {
		return resultRow{
			Year:         tc.Year,
			EntryID:      tc.EntryID,
			Dir:          tc.Dir,
			Title:        tc.Title,
			Award:        tc.Award,
			SourceRel:    tc.SourceRel,
			Category:     "preprocess_fail",
			PreprocessMS: pre.Duration.Milliseconds(),
			ErrorSummary: pre.ErrMsg,
		}
	}

	parseArgs := append(cArgs(systemIncludes, tc.Defines), "-x", "c99", "-parse-only", "-o", parseOut, srcPath)
	parse := runStage(timeout, compilerPath, parseArgs...)
	if !parse.OK {
		return resultRow{
			Year:         tc.Year,
			EntryID:      tc.EntryID,
			Dir:          tc.Dir,
			Title:        tc.Title,
			Award:        tc.Award,
			SourceRel:    tc.SourceRel,
			Category:     "parse_fail",
			PreprocessMS: pre.Duration.Milliseconds(),
			ParseMS:      parse.Duration.Milliseconds(),
			ErrorSummary: parse.ErrMsg,
		}
	}

	compArgs := append(cArgs(systemIncludes, tc.Defines), "-x", "c99", "-T", target, "-emit-ir", irOut, srcPath)
	comp := runStage(timeout, compilerPath, compArgs...)
	if !comp.OK {
		return resultRow{
			Year:         tc.Year,
			EntryID:      tc.EntryID,
			Dir:          tc.Dir,
			Title:        tc.Title,
			Award:        tc.Award,
			SourceRel:    tc.SourceRel,
			Category:     "compile_fail",
			PreprocessMS: pre.Duration.Milliseconds(),
			ParseMS:      parse.Duration.Milliseconds(),
			CompileMS:    comp.Duration.Milliseconds(),
			ErrorSummary: comp.ErrMsg,
		}
	}

	return resultRow{
		Year:         tc.Year,
		EntryID:      tc.EntryID,
		Dir:          tc.Dir,
		Title:        tc.Title,
		Award:        tc.Award,
		SourceRel:    tc.SourceRel,
		Category:     "ok",
		PreprocessMS: pre.Duration.Milliseconds(),
		ParseMS:      parse.Duration.Milliseconds(),
		CompileMS:    comp.Duration.Milliseconds(),
	}
}

func runStage(timeout time.Duration, compilerPath string, args ...string) stageOutcome {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	start := time.Now()
	cmd := exec.CommandContext(ctx, compilerPath, args...)
	out, err := cmd.CombinedOutput()
	dur := time.Since(start)
	if ctx.Err() == context.DeadlineExceeded {
		return stageOutcome{Name: args[0], Duration: dur, ErrMsg: "timeout"}
	}
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return stageOutcome{Name: args[0], Duration: dur, ErrMsg: summarizeErr(msg)}
	}
	return stageOutcome{Name: args[0], OK: true, Duration: dur}
}

func cArgs(systemIncludes []string, defines []string) []string {
	args := make([]string, 0, len(systemIncludes)*2+len(defines)*2)
	for _, def := range defines {
		args = append(args, "-D", def)
	}
	for _, dir := range systemIncludes {
		args = append(args, "-isystem", dir)
	}
	return args
}

func discoverEntryBuildFlags(entryDir string) ([]string, []string, error) {
	makefilePath := filepath.Join(entryDir, "Makefile")
	data, err := os.ReadFile(makefilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	vars := parseSimpleMakeVars(string(data))
	cdefine := expandMakeVars(vars["CDEFINE"], vars)
	cinclude := expandMakeVars(vars["CINCLUDE"], vars)
	defines := extractDefines(cdefine)
	defines = append(defines, extractRecipeDefines(string(data), vars)...)
	return uniqueStrings(defines), uniqueStrings(extractForcedIncludes(cinclude)), nil
}

func parseSimpleMakeVars(src string) map[string]string {
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	vars := make(map[string]string)
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if idx := strings.IndexByte(line, '#'); idx >= 0 {
			line = line[:idx]
		}
		line = strings.TrimRight(line, " \t")
		if strings.TrimSpace(line) == "" {
			continue
		}
		for strings.HasSuffix(line, "\\") && i+1 < len(lines) {
			line = strings.TrimRight(strings.TrimSuffix(line, "\\"), " \t") + " " + strings.TrimSpace(lines[i+1])
			i++
		}
		m := regexp.MustCompile(`^\s*([A-Za-z0-9_]+)\s*[:?+]?=\s*(.*)$`).FindStringSubmatch(line)
		if m == nil {
			continue
		}
		vars[m[1]] = strings.TrimSpace(m[2])
	}
	return vars
}

func expandMakeVars(s string, vars map[string]string) string {
	re := regexp.MustCompile(`\$\{([A-Za-z0-9_]+)\}|\$\(([A-Za-z0-9_]+)\)`)
	for i := 0; i < 16; i++ {
		changed := false
		s = re.ReplaceAllStringFunc(s, func(m string) string {
			sub := re.FindStringSubmatch(m)
			if len(sub) != 3 {
				return m
			}
			name := sub[1]
			if name == "" {
				name = sub[2]
			}
			if v, ok := vars[name]; ok {
				changed = true
				return v
			}
			return ""
		})
		if !changed {
			break
		}
	}
	return strings.TrimSpace(s)
}

func extractDefines(s string) []string {
	fields := splitShellWords(s)
	out := make([]string, 0, len(fields))
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if f == "-D" && i+1 < len(fields) {
			out = append(out, fields[i+1])
			i++
			continue
		}
		if strings.HasPrefix(f, "-D") && len(f) > 2 {
			out = append(out, f[2:])
		}
	}
	return out
}

func extractForcedIncludes(s string) []string {
	fields := splitShellWords(s)
	var out []string
	for i := 0; i < len(fields); i++ {
		if fields[i] == "-include" && i+1 < len(fields) {
			out = append(out, fields[i+1])
			i++
		}
	}
	return out
}

func extractRecipeDefines(src string, vars map[string]string) []string {
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	targets := make(map[string]bool)
	for _, name := range []string{
		expandMakeVars(vars["PROG"], vars),
		expandMakeVars(vars["ENTRY"], vars),
	} {
		for _, part := range splitShellWords(name) {
			if part != "" {
				targets[part] = true
			}
		}
	}
	if len(targets) == 0 {
		return nil
	}
	ruleRe := regexp.MustCompile(`^\s*([^:#=][^:]*)\s*:(.*)$`)
	var out []string
	inTargetRule := false
	for _, rawLine := range lines {
		line := strings.TrimRight(rawLine, "\r")
		if strings.HasPrefix(line, "\t") {
			if inTargetRule {
				out = append(out, extractDefines(expandMakeVars(strings.TrimSpace(line), vars))...)
			}
			continue
		}
		inTargetRule = false
		body := line
		if idx := strings.IndexByte(body, '#'); idx >= 0 {
			body = body[:idx]
		}
		body = strings.TrimSpace(body)
		if body == "" {
			continue
		}
		m := ruleRe.FindStringSubmatch(body)
		if m == nil {
			continue
		}
		for _, target := range splitShellWords(expandMakeVars(m[1], vars)) {
			if targets[target] {
				inTargetRule = true
				break
			}
		}
	}
	return out
}

func splitShellWords(s string) []string {
	var out []string
	var cur strings.Builder
	quote := byte(0)
	escaped := false
	flush := func() {
		if cur.Len() == 0 {
			return
		}
		out = append(out, cur.String())
		cur.Reset()
	}
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if escaped {
			cur.WriteByte(ch)
			escaped = false
			continue
		}
		if quote == '\'' {
			if ch == '\'' {
				quote = 0
			} else {
				cur.WriteByte(ch)
			}
			continue
		}
		if quote == '"' {
			switch ch {
			case '"':
				quote = 0
			case '\\':
				if i+1 < len(s) {
					i++
					cur.WriteByte(s[i])
				}
			default:
				cur.WriteByte(ch)
			}
			continue
		}
		switch ch {
		case '\\':
			escaped = true
		case '\'', '"':
			quote = ch
		case ' ', '\t', '\n', '\r':
			flush()
		default:
			cur.WriteByte(ch)
		}
	}
	flush()
	return out
}

func uniqueStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func writeWrapperSource(path string, headers []string, sourceAbs string) error {
	var b strings.Builder
	for _, h := range headers {
		fmt.Fprintf(&b, "#include <%s>\n", h)
	}
	fmt.Fprintf(&b, "#include %q\n", filepath.ToSlash(sourceAbs))
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func discoverHostSystemIncludes(cc string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, cc, "-E", "-Wp,-v", "-xc", "/dev/null")
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("timed out")
	}
	if err != nil {
		return nil, fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}

	lines := strings.Split(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n")
	var dirs []string
	var inList bool
	for _, line := range lines {
		if strings.Contains(line, "#include <...> search starts here:") {
			inList = true
			continue
		}
		if strings.Contains(line, "End of search list.") {
			break
		}
		if !inList {
			continue
		}
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "(framework directory)") {
			continue
		}
		dirs = append(dirs, line)
	}
	if len(dirs) == 0 {
		return nil, fmt.Errorf("no include directories found")
	}
	return dirs, nil
}

func summarizeErr(msg string) string {
	msg = strings.ReplaceAll(msg, "\r\n", "\n")
	lines := strings.Split(msg, "\n")
	var kept []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		kept = append(kept, line)
		if len(kept) == 3 {
			break
		}
	}
	out := strings.Join(kept, " | ")
	if len(out) > 320 {
		out = out[:320]
	}
	return out
}

func writeCSV(path string, rows []resultRow) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()

	header := []string{
		"year",
		"entry_id",
		"dir",
		"title",
		"award",
		"source_rel",
		"category",
		"preprocess_ms",
		"parse_ms",
		"compile_ms",
		"error_summary",
	}
	if err := w.Write(header); err != nil {
		return err
	}
	for _, row := range rows {
		record := []string{
			fmt.Sprintf("%d", row.Year),
			row.EntryID,
			row.Dir,
			row.Title,
			row.Award,
			row.SourceRel,
			row.Category,
			fmt.Sprintf("%d", row.PreprocessMS),
			fmt.Sprintf("%d", row.ParseMS),
			fmt.Sprintf("%d", row.CompileMS),
			row.ErrorSummary,
		}
		if err := w.Write(record); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}
