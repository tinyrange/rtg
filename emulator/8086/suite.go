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
	flagOF = uint16(1 << 11)
	flagSF = uint16(1 << 7)
	flagZF = uint16(1 << 6)

	knownFlagsMask = flagCF | flagOF | flagSF | flagZF
)

type SuiteOptions struct {
	SuiteDir       string
	DocumentedOnly bool
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

	for _, file := range files {
		key := suiteKeyFromFilename(file)
		status, flagsMask, hasFlagsMask := metadata.flagsForKey(key)
		if opts.DocumentedOnly && !isDocumentedStatus(status) {
			summary.FilesSkipped++
			continue
		}

		filePassed := 0
		fileFailed := 0
		fileUnsupported := 0

		if err := runSuiteFile(file, flagsMask, hasFlagsMask, opts, &summary, &filePassed, &fileFailed, &fileUnsupported, out); err != nil {
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

func runSuiteFile(file string, flagsMask uint16, hasFlagsMask bool, opts SuiteOptions, summary *SuiteSummary, filePassed, fileFailed, fileUnsupported *int, out io.Writer) error {
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
		summary.TestsRun++
		ok, unsupported, reason := runSuiteTest(test, flagsMask, hasFlagsMask, opts.Trace)
		if ok {
			summary.Passed++
			*filePassed = *filePassed + 1
		} else {
			summary.Failed++
			*fileFailed = *fileFailed + 1
			if unsupported {
				summary.Unsupported++
				*fileUnsupported = *fileUnsupported + 1
			}
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

func runSuiteTest(test suiteTest, flagsMask uint16, hasFlagsMask bool, trace bool) (bool, bool, string) {
	c := &cpu{trace: trace, maxSteps: 1}

	for _, pair := range test.Initial.RAM {
		c.mem[pair[0]&uint32(memSize-1)] = byte(pair[1])
	}
	if reason := applyInitialRegs(c, test.Initial.Regs); reason != "" {
		return false, false, reason
	}

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
		exp := byte(pair[1])
		if c.mem[addr] != exp {
			return false, false, fmt.Sprintf("memory[%05x] mismatch exp=%02x act=%02x", addr, exp, c.mem[addr])
		}
	}

	return true, false, ""
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
	c.cf = (flags & flagCF) != 0
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
