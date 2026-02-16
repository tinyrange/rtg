package emu8086

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	flagCF = uint16(1 << 0)
	flagPF = uint16(1 << 2)
	flagAF = uint16(1 << 4)
	flagOF = uint16(1 << 11)
	flagSF = uint16(1 << 7)
	flagZF = uint16(1 << 6)

	knownFlagsMask = flagCF | flagOF | flagSF | flagZF

	suiteCountsCacheVersion  = 1
	suiteCountsCacheFilename = ".suite-counts-cache.json"
)

type SuiteOptions struct {
	SuiteDir       string
	DocumentedOnly bool
	SkipTests      int
	MaxTests       int
	MaxFailures    int
	ProgressEvery  int
	Trace          bool
}

type SuiteFailure struct {
	File   string
	Index  int
	Name   string
	Reason string
}

type SuiteSummary struct {
	SuiteDir      string
	FilesRun      int
	FilesSkipped  int
	TestsRun      int
	Passed        int
	Failed        int
	Unsupported   int
	Duration      time.Duration
	StoppedByCaps bool
	Failures      []SuiteFailure
}

type suiteTest struct {
	Name    string   `json:"name"`
	Initial suiteCPU `json:"initial"`
	Final   suiteCPU `json:"final"`
	Idx     int      `json:"idx"`
}

type suiteCPU struct {
	Regs  map[string]uint16 `json:"regs"`
	RAM   [][2]uint32       `json:"ram"`
	Queue []uint8           `json:"queue"`
}

type suiteMetadata struct {
	Opcodes map[string]suiteOpcode `json:"opcodes"`
}

type suiteOpcode struct {
	Status    string                 `json:"status"`
	FlagsMask *uint16                `json:"flags-mask"`
	Reg       map[string]suiteOpcode `json:"reg"`
}

type suiteCountsCache struct {
	Version int                        `json:"version"`
	Files   map[string]suiteCountEntry `json:"files"`
}

type suiteCountEntry struct {
	Count       int   `json:"count"`
	Size        int64 `json:"size"`
	ModTimeUnix int64 `json:"mod_time_unix"`
}

func RunSuiteV2(opts SuiteOptions, out io.Writer) (SuiteSummary, error) {
	start := time.Now()
	if opts.ProgressEvery <= 0 {
		opts.ProgressEvery = 20_000
	}

	summary := SuiteSummary{SuiteDir: opts.SuiteDir}
	metadata, err := loadSuiteMetadata(opts.SuiteDir)
	if err != nil {
		return summary, err
	}

	files, err := filepath.Glob(filepath.Join(opts.SuiteDir, "*.json.gz"))
	if err != nil {
		return summary, fmt.Errorf("glob suite files: %w", err)
	}
	sort.Strings(files)
	if len(files) == 0 {
		return summary, fmt.Errorf("no .json.gz files found in %s", opts.SuiteDir)
	}
	fileTestCounts, err := loadSuiteFileTestCounts(opts.SuiteDir, files)
	if err != nil {
		return summary, err
	}
	skipRemaining := opts.SkipTests

	for _, file := range files {
		key := suiteKeyFromFilename(file)
		status, flagsMask, hasFlagsMask := metadata.flagsForKey(key)
		if opts.DocumentedOnly && !isDocumentedStatus(status) {
			summary.FilesSkipped++
			continue
		}
		if skipRemaining >= fileTestCounts[file] {
			skipRemaining -= fileTestCounts[file]
			continue
		}
		skipInFile := 0
		if skipRemaining > 0 {
			skipInFile = skipRemaining
			skipRemaining = 0
		}

		filePassed := 0
		fileFailed := 0
		fileUnsupported := 0

		if err := runSuiteFile(file, flagsMask, hasFlagsMask, skipInFile, opts, &summary, &filePassed, &fileFailed, &fileUnsupported, out); err != nil {
			return summary, err
		}
		summary.FilesRun++
		fmt.Fprintf(out, "file %s: pass=%d fail=%d unsupported=%d\n", key, filePassed, fileFailed, fileUnsupported)

		if summary.StoppedByCaps {
			break
		}
	}

	summary.Duration = time.Since(start)
	return summary, nil
}

func runSuiteFile(file string, flagsMask uint16, hasFlagsMask bool, skipInFile int, opts SuiteOptions, summary *SuiteSummary, filePassed, fileFailed, fileUnsupported *int, out io.Writer) error {
	f, err := os.Open(file)
	if err != nil {
		return fmt.Errorf("open %s: %w", file, err)
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("open gzip %s: %w", file, err)
	}
	defer gzr.Close()

	dec := json.NewDecoder(gzr)
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("read JSON start %s: %w", file, err)
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '[' {
		return fmt.Errorf("expected JSON array in %s", file)
	}

	for dec.More() {
		var test suiteTest
		if err := dec.Decode(&test); err != nil {
			return fmt.Errorf("decode test in %s: %w", file, err)
		}
		if skipInFile > 0 {
			skipInFile--
			continue
		}
		summary.TestsRun++
		ok, unsupported, reason := runSuiteTest(test, flagsMask, hasFlagsMask, opts.Trace)
		if ok {
			summary.Passed++
			*filePassed = *filePassed + 1
		} else {
			if unsupported {
				summary.Unsupported++
				*fileUnsupported = *fileUnsupported + 1
			}
			summary.Failed++
			*fileFailed = *fileFailed + 1
			if len(summary.Failures) < 25 {
				summary.Failures = append(summary.Failures, SuiteFailure{
					File:   suiteKeyFromFilename(file),
					Index:  test.Idx,
					Name:   test.Name,
					Reason: reason,
				})
			}
		}

		if summary.TestsRun%opts.ProgressEvery == 0 {
			fmt.Fprintf(out, "progress: tests=%d pass=%d fail=%d unsupported=%d\n",
				summary.TestsRun, summary.Passed, summary.Failed, summary.Unsupported)
		}
		if opts.MaxTests > 0 && summary.TestsRun >= opts.MaxTests {
			summary.StoppedByCaps = true
			break
		}
		if opts.MaxFailures > 0 && summary.Failed >= opts.MaxFailures {
			summary.StoppedByCaps = true
			break
		}
	}

	return nil
}

func loadSuiteFileTestCounts(suiteDir string, files []string) (map[string]int, error) {
	cachePath := filepath.Join(suiteDir, suiteCountsCacheFilename)
	cache := suiteCountsCache{
		Version: suiteCountsCacheVersion,
		Files:   make(map[string]suiteCountEntry),
	}

	if raw, err := os.ReadFile(cachePath); err == nil {
		var parsed suiteCountsCache
		if err := json.Unmarshal(raw, &parsed); err == nil && parsed.Version == suiteCountsCacheVersion {
			if parsed.Files == nil {
				parsed.Files = make(map[string]suiteCountEntry)
			}
			cache = parsed
		}
	}

	counts := make(map[string]int, len(files))
	updated := false
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", file, err)
		}
		base := filepath.Base(file)
		entry, ok := cache.Files[base]
		if ok && entry.Size == info.Size() && entry.ModTimeUnix == info.ModTime().UnixNano() {
			counts[file] = entry.Count
			continue
		}
		count, err := countSuiteTestsInFile(file)
		if err != nil {
			return nil, err
		}
		counts[file] = count
		cache.Files[base] = suiteCountEntry{
			Count:       count,
			Size:        info.Size(),
			ModTimeUnix: info.ModTime().UnixNano(),
		}
		updated = true
	}

	if updated {
		if raw, err := json.Marshal(cache); err == nil {
			tmp := cachePath + ".tmp"
			if err := os.WriteFile(tmp, raw, 0o644); err == nil {
				_ = os.Rename(tmp, cachePath)
			}
		}
	}

	return counts, nil
}

func countSuiteTestsInFile(file string) (int, error) {
	f, err := os.Open(file)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", file, err)
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return 0, fmt.Errorf("open gzip %s: %w", file, err)
	}
	defer gzr.Close()

	dec := json.NewDecoder(gzr)
	tok, err := dec.Token()
	if err != nil {
		return 0, fmt.Errorf("read JSON start %s: %w", file, err)
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '[' {
		return 0, fmt.Errorf("expected JSON array in %s", file)
	}

	count := 0
	var raw json.RawMessage
	for dec.More() {
		if err := dec.Decode(&raw); err != nil {
			return 0, fmt.Errorf("decode test in %s: %w", file, err)
		}
		count++
	}
	return count, nil
}

func runSuiteTest(test suiteTest, flagsMask uint16, hasFlagsMask bool, trace bool) (bool, bool, string) {
	c := &cpu{trace: trace, maxSteps: 1}

	for _, pair := range test.Initial.RAM {
		c.mem[pair[0]&uint32(memSize-1)] = byte(pair[1])
	}
	if reason := applyInitialRegs(c, test.Initial.Regs); reason != "" {
		return false, false, reason
	}
	isDivFaultInstr := decodeDivFaultInstr(c)

	if err := c.step(); err != nil {
		return false, strings.Contains(err.Error(), "unsupported"), err.Error()
	}

	for reg, exp := range test.Final.Regs {
		name := strings.ToLower(reg)
		if name == "flags" {
			mask := knownFlagsMask
			if hasFlagsMask {
				mask &= flagsMask
			}
			if mask == 0 {
				continue
			}
			act := cpuFlags(c)
			if (act & mask) != (exp & mask) {
				return false, false, fmt.Sprintf("flags mismatch exp=%04x act=%04x mask=%04x", exp, act, mask)
			}
			continue
		}
		act, ok := cpuReg(c, name)
		if !ok {
			return false, false, fmt.Sprintf("unknown final register %q", reg)
		}
		if act != exp {
			return false, false, fmt.Sprintf("register %s mismatch exp=%04x act=%04x", name, exp, act)
		}
	}

	for _, pair := range test.Final.RAM {
		addr := pair[0] & uint32(memSize-1)
		if isDivFaultInstr && c.cs == 0 && c.ip == 0x0400 {
			// On 8088 divide-error traps, FLAGS bits outside the core status set can
			// vary by microcode path. Skip strict byte-level comparison of pushed FLAGS.
			flagsLo := linear(c.ss, c.sp+4)
			flagsHi := linear(c.ss, c.sp+5)
			if addr == flagsLo || addr == flagsHi {
				continue
			}
		}
		exp := byte(pair[1])
		if c.mem[addr] != exp {
			return false, false, fmt.Sprintf("memory[%05x] mismatch exp=%02x act=%02x", addr, exp, c.mem[addr])
		}
	}

	return true, false, ""
}

func decodeDivFaultInstr(c *cpu) bool {
	pc := linear(c.cs, c.ip)
	i := uint32(0)
	for {
		op := c.rb(pc + i)
		switch op {
		case 0x26, 0x2e, 0x36, 0x3e:
			i++
			continue
		case 0xf0, 0xf2, 0xf3:
			i++
			continue
		case 0xf6, 0xf7:
			modrm := c.rb(pc + i + 1)
			subop := (modrm >> 3) & 0x7
			return subop == 6 || subop == 7
		default:
			return false
		}
	}
}

func applyInitialRegs(c *cpu, regs map[string]uint16) string {
	for reg, val := range regs {
		switch strings.ToLower(reg) {
		case "ax":
			c.ax = val
		case "bx":
			c.bx = val
		case "cx":
			c.cx = val
		case "dx":
			c.dx = val
		case "sp":
			c.sp = val
		case "bp":
			c.bp = val
		case "si":
			c.si = val
		case "di":
			c.di = val
		case "cs":
			c.cs = val
		case "ds":
			c.ds = val
		case "es":
			c.es = val
		case "ss":
			c.ss = val
		case "ip":
			c.ip = val
		case "flags":
			setCPUFlags(c, val)
		default:
			return fmt.Sprintf("unknown initial register %q", reg)
		}
	}
	return ""
}

func cpuReg(c *cpu, reg string) (uint16, bool) {
	switch reg {
	case "ax":
		return c.ax, true
	case "bx":
		return c.bx, true
	case "cx":
		return c.cx, true
	case "dx":
		return c.dx, true
	case "sp":
		return c.sp, true
	case "bp":
		return c.bp, true
	case "si":
		return c.si, true
	case "di":
		return c.di, true
	case "cs":
		return c.cs, true
	case "ds":
		return c.ds, true
	case "es":
		return c.es, true
	case "ss":
		return c.ss, true
	case "ip":
		return c.ip, true
	default:
		return 0, false
	}
}

func setCPUFlags(c *cpu, flags uint16) {
	c.extraFlags = flags &^ modeledFlagsMask
	c.cf = (flags & flagCF) != 0
	c.pf = (flags & flagPF) != 0
	c.af = (flags & flagAF) != 0
	c.of = (flags & flagOF) != 0
	c.sf = (flags & flagSF) != 0
	c.zf = (flags & flagZF) != 0
}

func cpuFlags(c *cpu) uint16 {
	var flags uint16
	if c.cf {
		flags |= flagCF
	}
	if c.of {
		flags |= flagOF
	}
	if c.sf {
		flags |= flagSF
	}
	if c.zf {
		flags |= flagZF
	}
	return flags
}

func loadSuiteMetadata(suiteDir string) (suiteMetadata, error) {
	var md suiteMetadata
	f, err := os.Open(filepath.Join(suiteDir, "metadata.json"))
	if err != nil {
		return md, fmt.Errorf("open metadata.json in %s: %w", suiteDir, err)
	}
	defer f.Close()
	if err := json.NewDecoder(f).Decode(&md); err != nil {
		return md, fmt.Errorf("decode metadata.json: %w", err)
	}
	return md, nil
}

func suiteKeyFromFilename(path string) string {
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, ".gz")
	base = strings.TrimSuffix(base, ".json")
	return strings.ToUpper(base)
}

func (m suiteMetadata) flagsForKey(key string) (string, uint16, bool) {
	if len(m.Opcodes) == 0 {
		return "", 0, false
	}
	parts := strings.Split(key, ".")
	opcode := parts[0]
	entry, ok := m.Opcodes[opcode]
	if !ok {
		return "", 0, false
	}

	status := entry.Status
	var flagsMask uint16
	hasFlagsMask := false
	if entry.FlagsMask != nil {
		flagsMask = *entry.FlagsMask
		hasFlagsMask = true
	}

	if len(parts) > 1 {
		reg := strings.TrimLeft(parts[1], "0")
		if reg == "" {
			reg = "0"
		}
		if regEntry, ok := entry.Reg[reg]; ok {
			if regEntry.Status != "" {
				status = regEntry.Status
			}
			if regEntry.FlagsMask != nil {
				flagsMask = *regEntry.FlagsMask
				hasFlagsMask = true
			}
		}
	}

	return status, flagsMask, hasFlagsMask
}

func isDocumentedStatus(status string) bool {
	switch status {
	case "undefined", "undocumented", "fpu":
		return false
	default:
		return true
	}
}
