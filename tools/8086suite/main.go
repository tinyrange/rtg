package main

import (
	"flag"
	"fmt"
	"os"

	emu8086 "j5.nz/rtg/emulator/8086"
)

func main() {
	suiteDir := flag.String("suite", detectDefaultSuiteDir(), "path to V2 suite directory")
	documentedOnly := flag.Bool("documented-only", true, "skip undefined/undocumented/FPU opcodes based on metadata")
	maxTests := flag.Int("max-tests", 0, "max tests to execute (0 means all)")
	maxFailures := flag.Int("max-failures", 0, "stop after this many failures (0 means no cap)")
	progressEvery := flag.Int("progress-every", 20_000, "print progress every N tests")
	trace := flag.Bool("trace", false, "trace every executed instruction")
	flag.Parse()

	fmt.Printf("suite: %s\n", *suiteDir)
	summary, err := emu8086.RunSuiteV2(emu8086.SuiteOptions{
		SuiteDir:       *suiteDir,
		DocumentedOnly: *documentedOnly,
		MaxTests:       *maxTests,
		MaxFailures:    *maxFailures,
		ProgressEvery:  *progressEvery,
		Trace:          *trace,
	}, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "suite run failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nsummary: suite=%s files_run=%d files_skipped=%d tests=%d pass=%d fail=%d unsupported=%d duration=%s stopped=%v\n",
		summary.SuiteDir, summary.FilesRun, summary.FilesSkipped, summary.TestsRun, summary.Passed, summary.Failed, summary.Unsupported, summary.Duration, summary.StoppedByCaps)
	if len(summary.Failures) > 0 {
		fmt.Printf("first failures:\n")
		for _, f := range summary.Failures {
			fmt.Printf("  %s idx=%d name=%q: %s\n", f.File, f.Index, f.Name, f.Reason)
		}
	}

	if summary.Failed > 0 {
		os.Exit(1)
	}
}

func detectDefaultSuiteDir() string {
	if dirExists("local/8086/v2") {
		return "local/8086/v2"
	}
	return "local/8088/v2"
}

func dirExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}
