package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func configureVMProgramArgs(entryFiles []string, programArgs []string) {
	// argv[0] is the program name, followed by actual args.
	vmArgs = append(vmArgs, "rtg")
	if len(programArgs) > 0 {
		vmArgs = append(vmArgs, programArgs...)
		return
	}
	i := 0
	for i < len(entryFiles) {
		vmArgs = append(vmArgs, entryFiles[i])
		i = i + 1
	}
}

func prepareRuntimeInputs(entryFiles []string, fromIRBinaryPath string, stdinInput bool, runMode bool, outputPath string, target CompilerTarget) ([]string, string, error) {
	if stdinInput {
		if fromIRBinaryPath != "" {
			return entryFiles, outputPath, fmt.Errorf("cannot use - with -from-ir-binary")
		}
		err := readStdinSourceToTemp()
		if err != nil {
			return entryFiles, outputPath, fmt.Errorf("rtg: failed to read stdin source: %v", err)
		}
		entryFiles = append(entryFiles, runTmpSrc)
	}

	if runMode {
		tmpDir := tempDirPath()
		sep := pathSep()
		pid := fmt.Sprintf("%d", os.Getpid())
		if runTmpSrc == "" {
			runTmpSrc = tmpDir + sep + "rtg-run-" + pid + ".go"
		}
		runTmpBin = tmpDir + sep + "rtg-run-" + pid
		if target.GOOS == "windows" {
			runTmpBin = runTmpBin + ".exe"
		}

		// Read from stdin if no entry files.
		if len(entryFiles) == 0 {
			err := readStdinSourceToTemp()
			if err != nil {
				return entryFiles, outputPath, fmt.Errorf("rtg -run: failed to read stdin source: %v", err)
			}
			entryFiles = append(entryFiles, runTmpSrc)
		}

		// Override output to temp binary.
		outputPath = runTmpBin
	}

	return entryFiles, outputPath, nil
}

func parseExitStatusFromErrorString(errStr string) (int, bool) {
	if !strings.HasPrefix(errStr, "exit status ") {
		return 0, false
	}
	codeStr := errStr[12:]
	code := 0
	j := 0
	for j < len(codeStr) {
		if codeStr[j] >= '0' && codeStr[j] <= '9' {
			code = code*10 + int(codeStr[j]-'0')
		}
		j++
	}
	return code, true
}

func runCompiledBinary(outputPath string) error {
	cmd := exec.Command(outputPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
