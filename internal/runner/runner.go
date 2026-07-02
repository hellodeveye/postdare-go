package runner

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/hellodeveye/postdare-go/internal/sse"
)

type CommandRunner interface {
	Run(ctx context.Context, taskID uint64, stage string, command string) error
}

type LocalCommandRunner struct {
	LogDir  string
	Timeout time.Duration
	Hub     *sse.Hub
	Logger  *zap.Logger
}

func (r *LocalCommandRunner) Run(parent context.Context, taskID uint64, stage string, command string) error {
	if command == "" {
		return nil
	}
	if r.Timeout == 0 {
		r.Timeout = 30 * time.Minute
	}
	ctx, cancel := context.WithTimeout(parent, r.Timeout)
	defer cancel()

	if err := os.MkdirAll(r.LogDir, 0o755); err != nil {
		return err
	}
	logPath := filepath.Join(r.LogDir, fmt.Sprintf("%d.log", taskID))
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	cmd := exec.CommandContext(ctx, "bash", "-lc", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	waitDone := make(chan struct{})
	go terminateProcessGroupOnCancel(ctx, cmd, waitDone)

	var mu sync.Mutex
	writeLine := func(line string) {
		formatted := fmt.Sprintf("[%s] %s\n", stage, line)
		mu.Lock()
		_, _ = file.WriteString(formatted)
		mu.Unlock()
		if r.Hub != nil {
			r.Hub.Publish(sse.DeployTopic(taskID), formatted)
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go scanPipe(stdout, &wg, writeLine)
	go scanPipe(stderr, &wg, writeLine)
	wg.Wait()

	err = cmd.Wait()
	close(waitDone)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		writeLine("command timeout")
		return fmt.Errorf("command timeout after %s", r.Timeout)
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		writeLine("command canceled")
		return fmt.Errorf("command canceled")
	}
	if err != nil {
		exitCode := -1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				exitCode = status.ExitStatus()
			}
		}
		writeLine(fmt.Sprintf("command exited with code %d", exitCode))
		return fmt.Errorf("command exited with code %d: %w", exitCode, err)
	}
	writeLine("stage command completed")
	return nil
}

func terminateProcessGroupOnCancel(ctx context.Context, cmd *exec.Cmd, waitDone <-chan struct{}) {
	<-ctx.Done()
	if cmd.Process == nil {
		return
	}
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		return
	}
	_ = syscall.Kill(-pgid, syscall.SIGTERM)
	select {
	case <-waitDone:
	case <-time.After(3 * time.Second):
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	}
}

func scanPipe(pipe interface{ Read([]byte) (int, error) }, wg *sync.WaitGroup, writeLine func(string)) {
	defer wg.Done()
	scanner := bufio.NewScanner(pipe)
	buffer := make([]byte, 0, 64*1024)
	scanner.Buffer(buffer, 1024*1024)
	for scanner.Scan() {
		writeLine(scanner.Text())
	}
}

func AppendLog(logFile string, hub *sse.Hub, taskID uint64, stage string, line string) {
	_ = os.MkdirAll(filepath.Dir(logFile), 0o755)
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err == nil {
		_, _ = f.WriteString(fmt.Sprintf("[%s] %s\n", stage, line))
		_ = f.Close()
	}
	if hub != nil {
		hub.Publish(sse.DeployTopic(taskID), fmt.Sprintf("[%s] %s\n", stage, line))
	}
}
