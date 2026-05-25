//go:build !windows && !linux

package ui

func startPlatformStream(id string, command string, cols int, rows int) (*PTYStream, error) {
	return startPipeStream(id, command)
}
