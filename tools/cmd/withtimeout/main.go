package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func main() {
	timeout := flag.Duration("timeout", 0, "hard timeout (e.g. 5s, 2m)")
	grace := flag.Duration("grace", 2*time.Second, "grace period after SIGTERM before SIGKILL")
	killWait := flag.Duration("kill-wait", 1*time.Second, "max wait after SIGKILL before exiting anyway")
	flag.Parse()

	if *timeout <= 0 {
		fmt.Fprintln(os.Stderr, "withtimeout: -timeout must be > 0")
		os.Exit(2)
	}
	if *grace < 0 {
		fmt.Fprintln(os.Stderr, "withtimeout: -grace must be >= 0")
		os.Exit(2)
	}
	if *killWait < 0 {
		fmt.Fprintln(os.Stderr, "withtimeout: -kill-wait must be >= 0")
		os.Exit(2)
	}
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: withtimeout -timeout 5s [-grace 2s] -- <command> [args...]")
		os.Exit(2)
	}

	args := flag.Args()
	if args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "withtimeout: missing command")
		os.Exit(2)
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "withtimeout: start failed: %v\n", err)
		os.Exit(1)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	timer := time.NewTimer(*timeout)
	defer timer.Stop()

	select {
	case err := <-done:
		exitWithChildStatus(err)
	case <-timer.C:
		pid := cmd.Process.Pid
		fmt.Fprintf(os.Stderr, "withtimeout: timeout after %s; sending SIGTERM to pid=%d\n", timeout.String(), pid)
		_ = signalGroup(pid, syscall.SIGTERM)

		graceTimer := time.NewTimer(*grace)
		defer graceTimer.Stop()

		select {
		case <-done:
			os.Exit(124)
		case <-graceTimer.C:
			fmt.Fprintf(os.Stderr, "withtimeout: grace expired after %s; sending SIGKILL to pid=%d\n", grace.String(), pid)
			_ = signalGroup(pid, syscall.SIGKILL)
			killTimer := time.NewTimer(*killWait)
			defer killTimer.Stop()
			select {
			case <-done:
			case <-killTimer.C:
				fmt.Fprintln(os.Stderr, "withtimeout: process did not exit after SIGKILL; exiting wrapper")
			}
			os.Exit(124)
		}
	}
}

func signalGroup(pid int, sig syscall.Signal) error {
	if pid <= 0 {
		return fmt.Errorf("invalid pid %d", pid)
	}
	// Negative pid targets the full process group.
	if err := syscall.Kill(-pid, sig); err == nil || err == syscall.ESRCH {
		return nil
	}
	return syscall.Kill(pid, sig)
}

func exitWithChildStatus(err error) {
	if err == nil {
		os.Exit(0)
	}
	var exitErr *exec.ExitError
	if ok := asExitError(err, &exitErr); ok {
		if ws, ok := exitErr.Sys().(syscall.WaitStatus); ok {
			if ws.Exited() {
				os.Exit(ws.ExitStatus())
			}
			if ws.Signaled() {
				os.Exit(128 + int(ws.Signal()))
			}
		}
	}
	fmt.Fprintf(os.Stderr, "withtimeout: wait failed: %v\n", err)
	os.Exit(1)
}

func asExitError(err error, out **exec.ExitError) bool {
	e, ok := err.(*exec.ExitError)
	if !ok {
		return false
	}
	*out = e
	return true
}
