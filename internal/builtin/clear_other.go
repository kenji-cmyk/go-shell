//go:build !windows

package builtin

import (
	"fmt"
	"io"
)

func clearScreen(out io.Writer, errOut io.Writer) error {
	_, err := fmt.Fprint(out, "\033[2J\033[H")
	return err
}
