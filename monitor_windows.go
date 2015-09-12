package panicwrap

import (
	"github.com/mitchellh/osext"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

func monitor(c *WrapConfig) (int, error) {
	return -1, fmt.Errorf("Monitor is not supported on windows")
}
