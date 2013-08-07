// The panicwrap package provides functions for capturing and handling
// panics in your application. It does this by re-executing the running
// application and monitoring stderr for any panics. At the same time,
// stdout/stderr/etc. are set to the same values so that data is shuttled
// through properly, making the existence of panicwrap mostly transparent.
//
// Panics are only detected when the subprocess exits with a non-zero
// exit status, since this is the only time panics are real. Otherwise,
// "panic-like" output is ignored.
package panicwrap

import (
	"bytes"
	"errors"
	"github.com/mitchellh/osext"
	"io"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

const (
	DEFAULT_COOKIE_KEY = "cccf35992f8f3cd8d1d28f0109dd953e26664531"
	DEFAULT_COOKIE_VAL = "7c28215aca87789f95b406b8dd91aa5198406750"
)

// HandlerFunc is the type called when a panic is detected.
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

	// Pipe the stderr so we can read all the data as we look for panics
	stderr_r, stderr_w := io.Pipe()
	doneCh := make(chan struct{})
	panicCh := make(chan string)

	// On close, make sure to finish off the copying of data to stderr
	defer func() {
		defer close(doneCh)
		stderr_w.Close()
		<-panicCh
	}()

	// Start the goroutine that will watch stderr for any panics
	go trackPanic(stderr_r, panicCh)

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
		return 1, err
	}

	// Listen to signals and capture them forever. We allow the child
	// process to handle them in some way.
	sigCh := make(chan os.Signal)
	signal.Notify(sigCh, os.Interrupt)
	go func() {
		defer signal.Stop(sigCh)
		for {
			select {
			case <-doneCh:
				return
			case <-sigCh:
			}
		}
	}()

	if err := cmd.Wait(); err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			// This is some other kind of subprocessing error.
			return 1, err
		}

		exitStatus := 1
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
			exitStatus = status.ExitStatus()
		}

		// Close the writer end so that the tracker goroutine ends at some point
		stderr_w.Close()

		// Wait on the panic data
		panicTxt := <-panicCh
		if panicTxt != "" {
			c.Handler(panicTxt)
		}

		return exitStatus, nil
	}

	return 0, nil
}

func trackPanic(r io.Reader, result chan<- string) {
	defer close(result)

	panicHeader := []byte("panic:")

	// Maintain a circular buffer of the data being read.
	buf := make([]byte, 2048)
	panicStart := -1
	cursor := 0
	readCursor := 0

	readPanicLen := func() int {
		if cursor < panicStart {
			// The cursor has wrapped around the end.
			return (len(buf) - panicStart) + cursor
		} else {
			return cursor - panicStart
		}
	}

	readPanicBytes := func() []byte {
		panicBytes := make([]byte, readPanicLen())
		if cursor < panicStart {
			copy(panicBytes, buf[panicStart:len(buf)])
			copy(panicBytes[len(buf)-panicStart:], buf[0:cursor])
		} else {
			copy(panicBytes, buf[panicStart:cursor])
		}

		return panicBytes
	}

	for {
		for panicStart < 0 && readCursor != cursor {
			// We're not currently tracking a panic, so we determine if
			// we have a panic by looking at the last handful of bytes.
			readCursorEnd := cursor
			if cursor < readCursor {
				readCursorEnd = len(buf)
			}

			inspectBuf := buf[readCursor:readCursorEnd]
			idx := bytes.Index(inspectBuf, panicHeader)
			if idx >= 0 {
				panicStart = readCursor + idx
				readCursorEnd = panicStart
			}

			// Write out the buffer we read to stderr to mirror it
			// through. If a panic started, we only write up to the
			// start of the panic.
			os.Stderr.Write(buf[readCursor:readCursorEnd])

			// Move the read cursor
			readCursor = readCursorEnd
			if readCursor > len(buf) {
				panic("read cursor past end of buffer")
			} else if readCursor == len(buf) {
				readCursor = 0
			}
		}

		if panicStart >= 0 && readPanicLen() >= 512 {
			// We're currently tracking a panic. If we've read at least
			// a certain number of bytes of the panic, verify if it is
			// a real panic. Otherwise, continue to just collect bytes.
			panicBytes := readPanicBytes()

			if !verifyPanic(panicBytes) {
				// Push the read cursor by at least one so we don't
				// infinite loop
				os.Stderr.Write(buf[panicStart : panicStart+1])
				readCursor += 1
				panicStart = -1
				continue
			}

			panicTxt := new(bytes.Buffer)
			panicTxt.Write(panicBytes)
			io.Copy(panicTxt, r)
			result <- panicTxt.String()
			return
		}

		// Read into the next portion of our buffer
		cursorEnd := cursor + int(math.Min(1024, float64(len(buf)-cursor)))
		n, err := r.Read(buf[cursor:cursorEnd])
		if n <= 0 {
			if err == nil {
				continue
			} else if err == io.EOF {
				result <- string(readPanicBytes())
				return
			}

			// TODO(mitchellh): handle errors?
		}

		cursor += n
		if cursor > len(buf) {
			panic("cursor past the end of the buffer")
		}

		if cursor == len(buf) {
			// Wrap around our buffer if we reached the end
			cursor = 0
		}
	}
}

func verifyPanic(p []byte) bool {
	return bytes.Index(p, []byte("goroutine ")) != -1
}
