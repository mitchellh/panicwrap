package panicwrap

import (
	"errors"
	"github.com/mitchellh/osext"
	"io"
	"os"
	"os/exec"
	"strings"
)

const (
	DEFAULT_COOKIE_KEY = "cccf35992f8f3cd8d1d28f0109dd953e26664531"
	DEFAULT_COOKIE_VAL = "7c28215aca87789f95b406b8dd91aa5198406750"
)

type HandlerFunc func(string)

// WrapConfig is the configuration for panicwrap when wrapping an existing
// binary. To get started, in general, you only need the BasicWrap function
// that will set this up for you. However, for more customizability,
// WrapConfig and Wrap can be used.
type WrapConfig struct {
	// Handler is the function called when a panic occurs.
	Handler HandlerFunc

	// The cookie key and value are used within environmental variables
	// to tell the child process that it is already executing so that
	// wrap doesn't re-wrap itself.
	CookieKey   string
	CookieValue string
}

// BasicWrap calls Wrap with the given handler function, using defaults
// for everything else. See Wrap and WrapConfig for more information on
// functionality and return values.
func BasicWrap(f HandlerFunc) (int, error) {
	return Wrap(&WrapConfig{
		Handler: f,
	})
}

// Wrap wraps the current executable in a handler to catch panics. It
// returns an error if there was an error during the wrapping process.
// If the error is nil, then the int result indicates the exit status of the
// child process. If the exit status is -1, then this is the child process,
// and execution should continue as normal. Otherwise, this is the parent
// process and the child successfully ran already, and you should exit the
// process with the returned exit status.
//
// This function should be called very very early in your program's execution.
// Ideally, this runs as the first line of code of main.
//
// Once this is called, the given WrapConfig shouldn't be modified or used
// any further.
func Wrap(c *WrapConfig) (int, error) {
	if c.Handler == nil {
		return -1, errors.New("Handler must be set")
	}

	if c.CookieKey == "" {
		c.CookieKey = DEFAULT_COOKIE_KEY
	}

	if c.CookieValue == "" {
		c.CookieValue = DEFAULT_COOKIE_VAL
	}

	// If the cookie key/value match our environment, then we are the
	// child, so just exit now and tell the caller that we're the child
	if os.Getenv(c.CookieKey) == c.CookieValue {
		return -1, nil
	}

	// Get the path to our current executable
	exePath, err := osext.Executable()
	if err != nil {
		return -1, err
	}

	stderr_r, stderr_w := io.Pipe()
	defer stderr_w.Close()

	// Start the goroutine that will watch stderr for any panics
	stderrDone := make(chan struct{})
	go func() {
		buf := make([]byte, 1024)

		for {
			n, err := stderr_r.Read(buf)
			if n > 0 {
				os.Stderr.Write(buf[0:n])
			}

			if err == io.EOF {
				break
			}

			// TODO(mitchellh): This can easily be far more efficient. One day.
			// TODO(mitchellh): doesn't handle buffer boundaries
			bufStr := string(buf[0:n])
			pIndex := strings.Index(bufStr, "panic:")
			if pIndex == -1 {
				continue
			}

			c.Handler("")
			break
		}

		close(stderrDone)
	}()

	// Build a subcommand to re-execute ourselves. We make sure to
	// set the environmental variable to include our cookie. We also
	// set stdin/stdout to match the config. Finally, we pipe stderr
	// through ourselves in order to watch for panics.
	cmd := exec.Command(exePath, os.Args[1:]...)
	cmd.Env = append(os.Environ(), c.CookieKey+"="+c.CookieValue)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = stderr_w
	if err := cmd.Start(); err != nil {
		return -1, err
	}

	// TODO(mitchellh): handle bad exit status
	cmd.Wait()
	stderr_w.Close()
	<-stderrDone

	return 0, nil
}
