//go:build windows

package ui

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	extendedStartupInfoPresent           = 0x00080000
	procThreadAttributePseudoConsole     = 0x00020016
	infinite                             = 0xffffffff
	conptyCreatePseudoConsoleUnsupported = syscall.ERROR_PROC_NOT_FOUND
	conptyResizePseudoConsoleUnsupported = syscall.ERROR_PROC_NOT_FOUND
	conptyClosePseudoConsoleUnsupported  = syscall.ERROR_PROC_NOT_FOUND
)

type coord struct {
	x int16
	y int16
}

var (
	kernel32                = windows.NewLazySystemDLL("kernel32.dll")
	procCreatePseudoConsole = kernel32.NewProc("CreatePseudoConsole")
	procResizePseudoConsole = kernel32.NewProc("ResizePseudoConsole")
	procClosePseudoConsole  = kernel32.NewProc("ClosePseudoConsole")
)

func startPlatformStream(id string, command string, cols int, rows int) (*PTYStream, error) {
	stream, err := startWindowsConPTYStream(id, command, cols, rows)
	if err == nil {
		return stream, nil
	}
	if isConPTYUnsupported(err) {
		return startPipeStream(id, command)
	}
	return nil, err
}

func startWindowsConPTYStream(id string, command string, cols int, rows int) (*PTYStream, error) {
	inputRead, inputWrite, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("create ConPTY input pipe: %w", err)
	}
	outputRead, outputWrite, err := os.Pipe()
	if err != nil {
		_ = inputRead.Close()
		_ = inputWrite.Close()
		return nil, fmt.Errorf("create ConPTY output pipe: %w", err)
	}

	pseudoConsole, err := createPseudoConsole(cols, rows, windows.Handle(inputRead.Fd()), windows.Handle(outputWrite.Fd()))
	if err != nil {
		_ = inputRead.Close()
		_ = inputWrite.Close()
		_ = outputRead.Close()
		_ = outputWrite.Close()
		return nil, err
	}
	_ = inputRead.Close()
	_ = outputWrite.Close()

	attributes, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		closePseudoConsole(pseudoConsole)
		_ = inputWrite.Close()
		_ = outputRead.Close()
		return nil, fmt.Errorf("create ConPTY attributes: %w", err)
	}
	if err := attributes.Update(procThreadAttributePseudoConsole, unsafe.Pointer(&pseudoConsole), unsafe.Sizeof(pseudoConsole)); err != nil {
		attributes.Delete()
		closePseudoConsole(pseudoConsole)
		_ = inputWrite.Close()
		_ = outputRead.Close()
		return nil, fmt.Errorf("attach ConPTY attribute: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	commandLine, err := windows.UTF16PtrFromString(windowsCommandLine(command))
	if err != nil {
		attributes.Delete()
		closePseudoConsole(pseudoConsole)
		_ = inputWrite.Close()
		_ = outputRead.Close()
		cancel()
		return nil, fmt.Errorf("build ConPTY command line: %w", err)
	}
	startupInfo := &windows.StartupInfoEx{
		StartupInfo: windows.StartupInfo{
			Cb: uint32(unsafe.Sizeof(windows.StartupInfoEx{})),
		},
		ProcThreadAttributeList: attributes.List(),
	}
	var processInfo windows.ProcessInformation
	if err := windows.CreateProcess(nil, commandLine, nil, nil, false, extendedStartupInfoPresent, nil, nil, &startupInfo.StartupInfo, &processInfo); err != nil {
		attributes.Delete()
		closePseudoConsole(pseudoConsole)
		_ = inputWrite.Close()
		_ = outputRead.Close()
		cancel()
		return nil, fmt.Errorf("start ConPTY process: %w", err)
	}
	attributes.Delete()
	_ = windows.CloseHandle(processInfo.Thread)

	stream := &PTYStream{
		id:      id,
		command: command,
		pty:     true,
		input:   inputWrite,
		resize: func(cols int, rows int) error {
			return resizePseudoConsole(pseudoConsole, cols, rows)
		},
		cancel: func() {
			cancel()
			_ = windows.TerminateProcess(processInfo.Process, 1)
		},
		messages: make(chan string, 128),
		done:     make(chan struct{}),
	}

	var readers sync.WaitGroup
	readers.Add(1)
	go copyStream(&readers, stream.messages, outputRead)
	go func() {
		waitForProcess(ctx, processInfo.Process)
		_ = inputWrite.Close()
		_ = outputRead.Close()
		readers.Wait()
		closePseudoConsole(pseudoConsole)
		_ = windows.CloseHandle(processInfo.Process)
		if ctx.Err() == nil {
			stream.messages <- "\n[process exited]\n"
		}
		close(stream.messages)
		close(stream.done)
		cancel()
	}()

	return stream, nil
}

func createPseudoConsole(cols int, rows int, input windows.Handle, output windows.Handle) (windows.Handle, error) {
	size := conPTYSize(cols, rows)
	var pseudoConsole windows.Handle
	r1, _, err := procCreatePseudoConsole.Call(
		uintptr(*(*uint32)(unsafe.Pointer(&size))),
		uintptr(input),
		uintptr(output),
		0,
		uintptr(unsafe.Pointer(&pseudoConsole)),
	)
	if r1 != 0 {
		return 0, fmt.Errorf("create ConPTY: %w", err)
	}
	return pseudoConsole, nil
}

func resizePseudoConsole(pseudoConsole windows.Handle, cols int, rows int) error {
	size := conPTYSize(cols, rows)
	r1, _, err := procResizePseudoConsole.Call(
		uintptr(pseudoConsole),
		uintptr(*(*uint32)(unsafe.Pointer(&size))),
	)
	if r1 != 0 {
		return fmt.Errorf("resize ConPTY: %w", err)
	}
	return nil
}

func closePseudoConsole(pseudoConsole windows.Handle) {
	if pseudoConsole == 0 {
		return
	}
	_, _, _ = procClosePseudoConsole.Call(uintptr(pseudoConsole))
}

func conPTYSize(cols int, rows int) coord {
	if cols <= 0 {
		cols = 100
	}
	if rows <= 0 {
		rows = 30
	}
	return coord{x: int16(cols), y: int16(rows)}
}

func waitForProcess(ctx context.Context, process windows.Handle) {
	done := make(chan struct{})
	go func() {
		_, _ = windows.WaitForSingleObject(process, infinite)
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		_ = windows.TerminateProcess(process, 1)
		<-done
	}
}

func windowsCommandLine(command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return defaultInteractiveCommand()
	}
	lower := strings.ToLower(command)
	comspec := strings.ToLower(defaultInteractiveCommand())
	if lower == "cmd" || lower == "cmd.exe" || lower == comspec {
		return command
	}
	return defaultInteractiveCommand() + " /C " + command
}

func isConPTYUnsupported(err error) bool {
	return strings.Contains(err.Error(), conptyCreatePseudoConsoleUnsupported.Error()) ||
		strings.Contains(err.Error(), conptyResizePseudoConsoleUnsupported.Error()) ||
		strings.Contains(err.Error(), conptyClosePseudoConsoleUnsupported.Error())
}

var _ io.WriteCloser = (*os.File)(nil)
