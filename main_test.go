package main

import (
	"os/exec"
	"testing"
)

func TestGhostCommitOutput(t *testing.T) {
	out, err := exec.Command("go", "run", ".").Output()
	if err != nil {
		t.Fatalf("failed to run ghost-commit: %v", err)
	}
	got := string(out)
	want := "hello world\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
