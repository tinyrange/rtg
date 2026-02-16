package main

import (
	"flag"
	"fmt"
	"os"

	emu8086 "j5.nz/rtg/emulator/8086"
)

func main() {
	trace := flag.Bool("trace", false, "trace executed instructions")
	maxSteps := flag.Int("max-steps", 2_000_000, "instruction step limit")
	flag.Parse()
	if flag.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "usage: %s [-trace] [-max-steps N] <program.com> [args...]\n", os.Args[0])
		os.Exit(2)
	}

	comPath := flag.Arg(0)
	progArgs := flag.Args()[1:]
	res, err := emu8086.RunCOMFile(comPath, progArgs, emu8086.Options{
		Trace:    *trace,
		MaxSteps: *maxSteps,
		DbgWrite: os.Getenv("COMEMU_DEBUG_WRITES") == "1",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "emulation error: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "comemu: exit=%d steps=%d int21_writes=%d\n", res.ExitCode, res.Steps, res.Int21Writes)
	os.Exit(res.ExitCode)
}
