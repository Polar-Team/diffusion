package deploy

import (
	"os/exec"
)

// buildExecCommand is a thin wrapper around exec.Command to allow tests to
// substitute a fake runner without touching the real exec package.
var buildExecCommand = func(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}
