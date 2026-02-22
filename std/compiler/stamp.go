package main

// compilerBuildGitHash can be set at compile time via -D main.compilerBuildGitHash=<hash>.
var compilerBuildGitHash = ""

func compilerStamp() string {
	if compilerBuildGitHash == "" {
		return "unknown"
	}
	return compilerBuildGitHash
}
