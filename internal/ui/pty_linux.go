//go:build linux

package ui

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

func startPlatformStream(id string, command string, cols int, rows int) (*PTYStream, error) {
	return startLinuxPTYStream(id, command, cols, rows)
}

func startLinuxPTYStream(id string, command string, cols int, rows int) (*PTYStream, error) {
	master, slave, err := openLinuxPTY(cols, rows)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "sh", "-lc", command)
	cmd.Stdin = slave
	cmd.Stdout = slave
	cmd.Stderr = slave
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
		Ctty:    int(slave.Fd()),
	}

	stream := &PTYStream{
		id:      id,
		command: command,
		pty:     true,
		input:   master,
		resize: func(cols int, rows int) error {
			return resizeLinuxPTY(master, cols, rows)
		},
		cancel:   cancel,
		messages: make(chan string, 128),
		done:     make(chan struct{}),
	}

	if err := cmd.Start(); err != nil {
		cancel()
		_ = master.Close()
		_ = slave.Close()
		return nil, fmt.Errorf("start PTY stream: %w", err)
	}
	_ = slave.Close()

	var readers sync.WaitGroup
	readers.Add(1)
	go copyStream(&readers, stream.messages, master)
	go func() {
		err := cmd.Wait()
		_ = master.Close()
		readers.Wait()
		if err != nil && ctx.Err() == nil {
			stream.messages <- fmt.Sprintf("\n[process exited: %v]\n", err)
		} else {
			stream.messages <- "\n[process exited]\n"
		}
		close(stream.messages)
		close(stream.done)
		cancel()
	}()

	return stream, nil
}

func resizeLinuxPTY(file *os.File, cols int, rows int) error {
	return unix.IoctlSetWinsize(int(file.Fd()), unix.TIOCSWINSZ, &unix.Winsize{
		Col: uint16(cols),
		Row: uint16(rows),
	})
}

func openLinuxPTY(cols int, rows int) (*os.File, *os.File, error) {
	masterFD, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("open PTY master: %w", err)
	}
	if err := unix.IoctlSetInt(masterFD, unix.TIOCSPTLCK, 0); err != nil {
		_ = unix.Close(masterFD)
		return nil, nil, fmt.Errorf("unlock PTY: %w", err)
	}
	number, err := unix.IoctlGetInt(masterFD, unix.TIOCGPTN)
	if err != nil {
		_ = unix.Close(masterFD)
		return nil, nil, fmt.Errorf("read PTY number: %w", err)
	}

	master := os.NewFile(uintptr(masterFD), "/dev/ptmx")
	slave, err := os.OpenFile("/dev/pts/"+strconv.Itoa(number), os.O_RDWR, 0)
	if err != nil {
		_ = master.Close()
		return nil, nil, fmt.Errorf("open PTY slave: %w", err)
	}
	if cols > 0 && rows > 0 {
		_ = resizeLinuxPTY(master, cols, rows)
	}
	return master, slave, nil
}

var _ io.ReadWriteCloser = (*os.File)(nil)
