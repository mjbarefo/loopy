package loop

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTailFileSmallFileWhole(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log")
	if err := os.WriteFile(path, []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	data, truncated, size, err := TailFile(path, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if truncated || size != 8 || string(data) != "one\ntwo\n" {
		t.Fatalf("got truncated=%v size=%d data=%q", truncated, size, data)
	}
}

func TestTailFileTruncatesToWholeLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log")
	content := strings.Repeat("0123456789\n", 100) // 1100 bytes
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	data, truncated, size, err := TailFile(path, 105)
	if err != nil {
		t.Fatal(err)
	}
	if !truncated {
		t.Fatal("expected truncation")
	}
	if size != int64(len(content)) {
		t.Fatalf("size = %d, want %d", size, len(content))
	}
	if bytes.HasPrefix(data, []byte("123")) || data[0] != '0' {
		t.Fatalf("tail should start at a line boundary, got %q", data[:10])
	}
	if len(data) > 105 {
		t.Fatalf("tail longer than cap: %d", len(data))
	}
}

func TestTailFileMissing(t *testing.T) {
	_, _, _, err := TailFile(filepath.Join(t.TempDir(), "absent"), 1024)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("want os.ErrNotExist, got %v", err)
	}
}
