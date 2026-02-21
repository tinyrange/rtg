package main

type ExitError struct {
	Code int
}

func (e *ExitError) Error() string { return "exit" }

func maybeWrap(err error) error {
	return err
}

func handleRunError(err error) int {
	err = maybeWrap(err)
	if ee, ok := err.(*ExitError); ok {
		_ = ee
		return 1
	}
	if err != nil {
		return 2
	}
	return 0
}

func main() {
	_ = handleRunError(&ExitError{Code: 1}) // Repro shape for helperized type assertion instability.
}
