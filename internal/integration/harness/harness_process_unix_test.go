//go:build !windows

package harness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestHarness_CloseTerminatesSpawnedBackgroundChild(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")

	h := New(Config{
		LaunchCommand: fmt.Sprintf("sleep 60 & echo $! > %q; wait", pidFile),
		TranscriptDir: t.TempDir(),
	})
	if err := h.startLaunchCommand(context.Background()); err != nil {
		t.Fatalf("startLaunchCommand failed: %v", err)
	}

	pid := waitForChildPID(t, pidFile)
	if !processExists(pid) {
		t.Fatalf("expected spawned background child pid %d to exist before Close", pid)
	}

	if err := h.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for processExists(pid) {
		if time.Now().After(deadline) {
			t.Fatalf("background child pid %d still alive after harness close", pid)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func waitForChildPID(t *testing.T, pidFile string) int {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		data, err := os.ReadFile(pidFile)
		if err != nil {
			if time.Now().After(deadline) {
				t.Fatalf("timed out waiting for child pid file: %v", err)
			}
			time.Sleep(10 * time.Millisecond)
			continue
		}
		text := strings.TrimSpace(string(data))
		pid, err := strconv.Atoi(text)
		if err != nil || pid <= 0 {
			t.Fatalf("invalid pid in %s: %q (%v)", pidFile, text, err)
		}
		return pid
	}
}

func processExists(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
