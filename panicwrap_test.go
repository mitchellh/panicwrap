package panicwrap

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"testing"
)

func helperProcess(s ...string) *exec.Cmd {
	cs := []string{"-test.run=TestHelperProcess", "--"}
	cs = append(cs, s...)
	env := []string{
		"GO_WANT_HELPER_PROCESS=1",
	}

	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = append(env, os.Environ()...)
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	return cmd
}

// This is executed by `helperProcess` in a separate process in order to
// provider a proper sub-process environment to test some of our functionality.
func TestHelperProcess(*testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	// Find the arguments to our helper, which are the arguments past
	// the "--" in the command line.
	args := os.Args
	for len(args) > 0 {
		if args[0] == "--" {
			args = args[1:]
			break
		}

		args = args[1:]
	}

	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "No command\n")
		os.Exit(2)
	}

	cmd, args := args[0], args[1:]
	switch cmd {
	case "no-panic-output":
		fmt.Fprint(os.Stdout, "i am output")
		fmt.Fprint(os.Stderr, "stderr out")
		os.Exit(0)
	case "panic":
		exitStatus, err := BasicWrap(func(s string) {
			fmt.Fprintf(os.Stdout, "wrapped: %d", len(s))
			os.Exit(0)
		})

		if err != nil {
			fmt.Fprintf(os.Stderr, "wrap error: %s", err)
			os.Exit(1)
		}

		if exitStatus < 0 {
			panic("uh oh")
		}

		os.Exit(exitStatus)
	case "signal":
		exitStatus, err := BasicWrap(func(s string) {
			fmt.Fprintf(os.Stdout, "wrapped: %d", len(s))
			os.Exit(0)
		})

		if err != nil {
			fmt.Fprintf(os.Stderr, "wrap error: %s", err)
			os.Exit(1)
		}

		if exitStatus < 0 {
			c := make(chan os.Signal)
			signal.Notify(c, os.Interrupt)
			<-c
			fmt.Fprintf(os.Stdout, "got sigint")
			exitStatus = 0
		}

		os.Exit(exitStatus)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %q\n", cmd)
		os.Exit(2)
	}
}

func TestPanicWrap_Output(t *testing.T) {
	stderr := new(bytes.Buffer)
	stdout := new(bytes.Buffer)

	p := helperProcess("no-panic-output")
	p.Stdout = stdout
	p.Stderr = stderr
	if err := p.Run(); err != nil {
		t.Fatalf("err: %s", err)
	}

	if !strings.Contains(stdout.String(), "i am output") {
		t.Fatalf("didn't forward: %#v", stdout.String())
	}

	if !strings.Contains(stderr.String(), "stderr out") {
		t.Fatalf("didn't forward: %#v", stderr.String())
	}
}

func TestPanicWrap_Wrap(t *testing.T) {
	stdout := new(bytes.Buffer)

	p := helperProcess("panic")
	p.Stdout = stdout
	if err := p.Run(); err != nil {
		t.Fatalf("err: %s", err)
	}

	if !strings.Contains(stdout.String(), "wrapped: 1005") {
		t.Fatalf("didn't wrap: %#v", stdout.String())
	}
}
