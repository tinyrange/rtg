package main

type runErr struct{}

func (e runErr) Error() string { return "failed" }

func runMode() error {
	return runErr{}
}

func report(err error) string {
	if err == nil {
		return "ok"
	}
	return err.Error() // Repro shape for unresolved calls: err.Error.
}

func main() {
	_ = report(runMode())
}
