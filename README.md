# panicwrap

panicwrap is a Go library that re-executes a Go binary and monitors stderr
output from the binary for a panic. When it find a panic, it executes a
user-defined handler function. Stdout, stderr, stdin, signals, and exit
codes continue to work as normal, making the existence of panicwrap mostly
invisble to the end user until a panic actually occurs.

While globally catching panics within a Go application is not supposed to be
done, since a panic is truly a bug in the program meant to crash the system,
it is often useful to have a way to know when these panics occur. panicwrap
allows you to do something with these panics, such as writing them to a file,
so that you can track when panics occur.

Because it would be sad to panic while capturing a panic, it is recommended
that the handler functions for panicwrap remain relatively simple and well
tested. panicwrap itself contains many tests.

## Features

* **SIMPLE!**.
* Works with all Go applications
* Custom behavior when a panic occurs
* Stdout, stderr, stdin, exit codes, and signals continue to work as
  expected.

## Usage

In your main application:

```go
package main

import (
	"fmt"
	"github.com/mitchellh/panicwrap"
	"os"
)

func main() {
	exitStatus, err := panicwrap.BasicWrap(panicHandler)
	if err != nil {
		// Something went wrong setting up the panic wrapper. Unlikely,
		// but possible.
		panic(err)
	}

	// If exitStatus >= 0, then we're the parent process and the panicwrap
	// re-executed ourselves and completed. Just exit with the proper status.
	if exitStatus >= 0 {
		os.Exit(exitStatus)
	}

	// Otherwise, exitStatus < 0 means we're the child. Continue executing as
	// normal...

	// Let's say we panic
	panic("oh shucks")
}

func panicHandler(output string) {
	// output contains the full output (including stack traces) of the
	// panic. Put it in a file or something.
	fmt.Printf("The child panicked:\n\n%s\n", output)
	os.Exit(1)
}
```
