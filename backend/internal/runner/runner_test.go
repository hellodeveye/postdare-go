package runner

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestLocalCommandRunnerCancelsProcessGroup(t *testing.T) {
	tmp := t.TempDir()
	pidFile := filepath.Join(tmp, "child.pid")
	r := &LocalCommandRunner{
		LogDir:  filepath.Join(tmp, "logs"),
		Timeout: time.Minute,
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- r.Run(ctx, 1, "test", "sh -c 'sleep 30 & echo $! > \""+pidFile+"\"; wait'")
	}()

	var childPID int
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(pidFile)
		if err == nil {
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(raw)))
			if parseErr == nil && pid > 0 {
				childPID = pid
				break
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	if childPID == 0 {
		cancel()
		t.Fatal("child process pid was not written")
	}

	cancel()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "command canceled") {
			t.Fatalf("expected command canceled error, got %v", err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("runner did not return after cancellation")
	}

	deadline = time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if !processExists(childPID) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("child process %d still exists after cancel", childPID)
}

func processExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil
}
