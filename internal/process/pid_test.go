package process

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPIDExecutablePath(t *testing.T) {
	t.Parallel()

	pid := PID(os.Getpid())

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}

	path, err := pidExecutablePath(pid)
	if err != nil {
		t.Fatalf("pidExecutablePath(%d): %v", pid, err)
	}
	if path != exe {
		t.Errorf("ExecutablePath = %q, want %q", path, exe)
	}
}

func TestPIDName(t *testing.T) {
	t.Parallel()

	pid := PID(os.Getpid())

	path, err := pidExecutablePath(pid)
	if err != nil {
		t.Fatalf("pidExecutablePath(%d): %v", pid, err)
	}

	name, err := pidName(pid, path)
	if err != nil {
		t.Fatalf("pidName(%d): %v", pid, err)
	}
	if name == "" {
		t.Errorf("Name() = %q, want non-empty name", name)
	}
}
