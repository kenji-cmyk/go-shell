package executor

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go-shell/internal/parser"
)

type Executor struct {
	In   io.Reader
	Out  io.Writer
	Err  io.Writer
	Jobs *JobTable
}

type JobStatus string

const (
	JobRunning JobStatus = "running"
	JobDone    JobStatus = "done"
	JobFailed  JobStatus = "failed"
)

type Job struct {
	ID       int
	Command  string
	PIDs     []int
	Status   JobStatus
	Err      error
	Started  time.Time
	Finished time.Time
	done     chan struct{}
}

type JobSnapshot struct {
	ID       int
	Command  string
	PIDs     []int
	Status   JobStatus
	Err      error
	Started  time.Time
	Finished time.Time
}

type JobTable struct {
	mu     sync.Mutex
	nextID int
	jobs   map[int]*Job
}

func NewJobTable() *JobTable {
	return &JobTable{
		nextID: 1,
		jobs:   make(map[int]*Job),
	}
}

func (t *JobTable) Add(command string, processes []*exec.Cmd) *JobSnapshot {
	t.mu.Lock()
	defer t.mu.Unlock()

	pids := make([]int, 0, len(processes))
	for _, process := range processes {
		if process.Process != nil {
			pids = append(pids, process.Process.Pid)
		}
	}

	job := &Job{
		ID:      t.nextID,
		Command: command,
		PIDs:    pids,
		Status:  JobRunning,
		Started: time.Now(),
		done:    make(chan struct{}),
	}
	t.jobs[job.ID] = job
	t.nextID++

	snapshot := job.snapshot()
	return &snapshot
}

func (t *JobTable) Complete(id int, err error) {
	t.mu.Lock()
	job, ok := t.jobs[id]
	if !ok {
		t.mu.Unlock()
		return
	}
	if err != nil {
		job.Status = JobFailed
		job.Err = err
	} else {
		job.Status = JobDone
	}
	job.Finished = time.Now()
	close(job.done)
	t.mu.Unlock()
}

func (t *JobTable) List() []JobSnapshot {
	t.mu.Lock()
	defer t.mu.Unlock()

	jobs := make([]JobSnapshot, 0, len(t.jobs))
	for _, job := range t.jobs {
		snapshot := job.snapshot()
		jobs = append(jobs, snapshot)
	}
	return jobs
}

func (t *JobTable) Get(id int) (JobSnapshot, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	job, ok := t.jobs[id]
	if !ok {
		return JobSnapshot{}, false
	}
	return job.snapshot(), true
}

func (t *JobTable) Wait(id int) (JobSnapshot, error) {
	t.mu.Lock()
	job, ok := t.jobs[id]
	if !ok {
		t.mu.Unlock()
		return JobSnapshot{}, fmt.Errorf("job %d not found", id)
	}
	done := job.done
	t.mu.Unlock()

	<-done
	snapshot, ok := t.Get(id)
	if !ok {
		return JobSnapshot{}, fmt.Errorf("job %d not found", id)
	}
	return snapshot, nil
}

func (j *Job) snapshot() JobSnapshot {
	return JobSnapshot{
		ID:       j.ID,
		Command:  j.Command,
		PIDs:     append([]int(nil), j.PIDs...),
		Status:   j.Status,
		Err:      j.Err,
		Started:  j.Started,
		Finished: j.Finished,
	}
}

func (e Executor) Run(cmd parser.Command) error {
	if cmd.Name == "" {
		return nil
	}

	process := commandProcess(cmd)

	process.Stdin = e.In
	process.Stdout = e.Out
	process.Stderr = e.Err

	if err := process.Run(); err != nil {
		var notFound *exec.Error
		if errors.As(err, &notFound) && errors.Is(notFound.Err, exec.ErrNotFound) {
			return fmt.Errorf("%s: command not found", cmd.Name)
		}
		return err
	}
	return nil
}

func (e Executor) RunLine(line parser.Line) error {
	if len(line.Commands) == 0 {
		return nil
	}
	if len(line.Commands) == 1 && line.InputRedirect == "" && line.OutputRedirect == "" && !line.Background {
		return e.Run(line.Commands[0])
	}

	var input io.Reader = e.In
	var output io.Writer = e.Out
	var closers []io.Closer

	if line.InputRedirect != "" {
		file, err := os.Open(line.InputRedirect)
		if err != nil {
			return fmt.Errorf("open input redirect: %w", err)
		}
		closers = append(closers, file)
		input = file
	}

	if line.OutputRedirect != "" {
		flag := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
		if line.AppendOutput {
			flag = os.O_CREATE | os.O_WRONLY | os.O_APPEND
		}
		file, err := os.OpenFile(line.OutputRedirect, flag, 0644)
		if err != nil {
			return fmt.Errorf("open output redirect: %w", err)
		}
		closers = append(closers, file)
		output = file
	}
	closeFiles := func() {
		for _, closer := range closers {
			_ = closer.Close()
		}
	}

	processes := make([]*exec.Cmd, len(line.Commands))
	for i, command := range line.Commands {
		processes[i] = commandProcess(command)
		processes[i].Stderr = e.Err
	}

	processes[0].Stdin = input
	processes[len(processes)-1].Stdout = output

	readers := make([]*io.PipeReader, 0, len(processes)-1)
	writers := make([]*io.PipeWriter, 0, len(processes)-1)
	for i := 0; i < len(processes)-1; i++ {
		reader, writer := io.Pipe()
		processes[i].Stdout = writer
		processes[i+1].Stdin = reader
		readers = append(readers, reader)
		writers = append(writers, writer)
	}

	for _, process := range processes {
		if err := process.Start(); err != nil {
			closeFiles()
			return commandStartError(process, err)
		}
	}

	if line.Background {
		var job *JobSnapshot
		if e.Jobs != nil {
			job = e.Jobs.Add(line.Raw, processes)
			fmt.Fprintf(e.Out, "[%d]", job.ID)
			for _, pid := range job.PIDs {
				fmt.Fprintf(e.Out, " %d", pid)
			}
		} else {
			fmt.Fprintf(e.Out, "[background]")
			for _, process := range processes {
				if process.Process != nil {
					fmt.Fprintf(e.Out, " %d", process.Process.Pid)
				}
			}
		}
		fmt.Fprintln(e.Out)

		go func() {
			err := waitProcesses(processes, readers, writers)
			closeFiles()
			if e.Jobs != nil && job != nil {
				e.Jobs.Complete(job.ID, err)
			}
		}()
		return nil
	}

	defer closeFiles()
	return waitProcesses(processes, readers, writers)
}

func waitProcesses(processes []*exec.Cmd, readers []*io.PipeReader, writers []*io.PipeWriter) error {
	errs := make(chan error, len(processes))
	for i, process := range processes {
		go func(i int, process *exec.Cmd) {
			err := process.Wait()
			if i < len(writers) {
				_ = writers[i].Close()
			}
			if i > 0 {
				_ = readers[i-1].Close()
			}
			errs <- err
		}(i, process)
	}

	var firstErr error
	for range processes {
		if err := <-errs; err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

func commandProcess(cmd parser.Command) *exec.Cmd {
	if shouldFallbackToCMD(cmd.Name) {
		return exec.Command("cmd", "/C", cmd.Raw)
	}
	return exec.Command(cmd.Name, expandWildcards(cmd.Args)...)
}

func commandStartError(process *exec.Cmd, err error) error {
	var notFound *exec.Error
	if errors.As(err, &notFound) && errors.Is(notFound.Err, exec.ErrNotFound) {
		return fmt.Errorf("%s: command not found", process.Args[0])
	}
	return err
}

func expandWildcards(args []string) []string {
	var expanded []string
	for _, arg := range args {
		if !strings.ContainsAny(arg, "*?") {
			expanded = append(expanded, arg)
			continue
		}

		matches, err := filepath.Glob(arg)
		if err != nil || len(matches) == 0 {
			expanded = append(expanded, arg)
			continue
		}
		expanded = append(expanded, matches...)
	}
	return expanded
}

func shouldFallbackToCMD(name string) bool {
	switch strings.ToLower(name) {
	case "dir", "cls", "copy", "del", "type":
		return true
	default:
		return false
	}
}
