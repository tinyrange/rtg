package main

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
