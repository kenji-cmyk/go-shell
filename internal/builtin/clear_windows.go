//go:build windows

package builtin

import (
	"io"
	"os/exec"
)

func clearScreen(out io.Writer, errOut io.Writer) error {
	clear := exec.Command("cmd", "/C", "cls")
	clear.Stdout = out
	clear.Stderr = errOut
	return clear.Run()
}
