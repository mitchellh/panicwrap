package panicwrap

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
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

	defer os.Exit(0)

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
	case "panic":
		exitStatus, err := BasicWrap(func(string) {
			fmt.Fprint(os.Stdout, "wrapped")
		})

		if err != nil {
			fmt.Fprintf(os.Stderr, "wrap error: %s", err)
			os.Exit(1)
		}

		if exitStatus < 0 {
			panic("A PANIC")
		}

		os.Exit(exitStatus)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %q\n", cmd)
		os.Exit(2)
	}
}

func TestPanicWrap(t *testing.T) {
	stdout := new(bytes.Buffer)

	p := helperProcess("panic")
	p.Stdout = stdout
	if err := p.Run(); err != nil {
		t.Fatalf("err: %s", err)
	}

	if stdout.String() != "wrapped" {
		t.Fatal("didn't wrap")
	}
}
