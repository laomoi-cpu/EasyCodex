package wezterm

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGetTextRequiresWeztermExecutable(t *testing.T) {
	cli := CLI{Root: t.TempDir(), Timeout: time.Second}
	_, err := cli.GetText(context.Background(), "easyterm", "1", 100, false)
	if err == nil {
		t.Fatalf("expected missing executable error")
	}
}

func TestMuxDirFallsBack(t *testing.T) {
	dir := t.TempDir()
	if got := muxDir(dir); got == "" {
		t.Fatalf("muxDir returned empty path")
	}
}

func TestListRequiresClass(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "wezterm.exe"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	cli := CLI{Root: root, Timeout: time.Second}
	_, err := cli.List(context.Background(), "")
	if err == nil {
		t.Fatalf("expected class error")
	}
}
