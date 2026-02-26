package sshconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindHostBlock(t *testing.T) {
	lines := strings.Split(`# Global settings
ServerAliveInterval 60

# stm:group=prod
Host my-server
    HostName example.com
    User deploy

Host other
    HostName other.com`, "\n")

	block := FindHostBlock(lines, "my-server")
	if block == nil {
		t.Fatal("expected to find my-server block")
	}
	if block.StartLine != 3 {
		t.Errorf("StartLine = %d, want 3 (includes stm comment)", block.StartLine)
	}
	if block.EndLine != 8 {
		t.Errorf("EndLine = %d, want 8", block.EndLine)
	}

	block2 := FindHostBlock(lines, "other")
	if block2 == nil {
		t.Fatal("expected to find other block")
	}
	if block2.StartLine != 8 {
		t.Errorf("StartLine = %d, want 8", block2.StartLine)
	}

	block3 := FindHostBlock(lines, "nonexistent")
	if block3 != nil {
		t.Error("expected nil for nonexistent host")
	}
}

func TestRemoveHostBlock(t *testing.T) {
	lines := strings.Split(`Host first
    HostName first.com

Host second
    HostName second.com

Host third
    HostName third.com`, "\n")

	result, err := RemoveHostBlock(lines, "second")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	joined := strings.Join(result, "\n")
	if strings.Contains(joined, "second") {
		t.Errorf("second should be removed:\n%s", joined)
	}
	if !strings.Contains(joined, "first") {
		t.Errorf("first should remain:\n%s", joined)
	}
	if !strings.Contains(joined, "third") {
		t.Errorf("third should remain:\n%s", joined)
	}
}

func TestRemoveHostBlockWithMetadata(t *testing.T) {
	lines := strings.Split(`Host first
    HostName first.com

# stm:group=prod
# stm:autoconnect=true
Host second
    HostName second.com

Host third
    HostName third.com`, "\n")

	result, err := RemoveHostBlock(lines, "second")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	joined := strings.Join(result, "\n")
	if strings.Contains(joined, "stm:group=prod") {
		t.Errorf("metadata should be removed:\n%s", joined)
	}
	if strings.Contains(joined, "second") {
		t.Errorf("second should be removed:\n%s", joined)
	}
}

func TestRemoveHostBlockNotFound(t *testing.T) {
	lines := []string{"Host only", "    HostName only.com"}
	_, err := RemoveHostBlock(lines, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent host")
	}
}

func TestReplaceHostBlock(t *testing.T) {
	lines := strings.Split(`Host first
    HostName first.com

Host second
    HostName second.com
    User old

Host third
    HostName third.com`, "\n")

	newContent := "# stm:group=new\nHost second\n    HostName new.com\n    User new\n"

	result, err := ReplaceHostBlock(lines, "second", newContent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	joined := strings.Join(result, "\n")
	if !strings.Contains(joined, "HostName new.com") {
		t.Errorf("should contain new content:\n%s", joined)
	}
	if strings.Contains(joined, "User old") {
		t.Errorf("old content should be replaced:\n%s", joined)
	}
	if !strings.Contains(joined, "first") {
		t.Errorf("first should remain:\n%s", joined)
	}
	if !strings.Contains(joined, "third") {
		t.Errorf("third should remain:\n%s", joined)
	}
}

func TestAppendHostBlock(t *testing.T) {
	lines := strings.Split(`Host existing
    HostName existing.com`, "\n")

	result := AppendHostBlock(lines, "Host new-one\n    HostName new.com\n")

	joined := strings.Join(result, "\n")
	if !strings.Contains(joined, "Host new-one") {
		t.Errorf("should contain appended block:\n%s", joined)
	}
	if !strings.Contains(joined, "Host existing") {
		t.Errorf("existing should remain:\n%s", joined)
	}
}

func TestAppendHostBlockToEmpty(t *testing.T) {
	var lines []string
	result := AppendHostBlock(lines, "Host new-one\n    HostName new.com\n")

	if len(result) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(result))
	}
	if result[0] != "Host new-one" {
		t.Errorf("first line = %q, want %q", result[0], "Host new-one")
	}
}

func TestReadWriteLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")

	original := []string{
		"Host test",
		"    HostName test.com",
		"    User admin",
	}

	if err := WriteLines(path, original); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := ReadLines(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if len(got) != len(original) {
		t.Fatalf("line count = %d, want %d", len(got), len(original))
	}
	for i, line := range got {
		if line != original[i] {
			t.Errorf("line %d = %q, want %q", i, line, original[i])
		}
	}
}

func TestReadLinesMissingFile(t *testing.T) {
	lines, err := ReadLines(filepath.Join(t.TempDir(), "nonexistent"))
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if lines != nil {
		t.Fatalf("expected nil lines, got %v", lines)
	}
}

func TestReadLinesEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	lines, err := ReadLines(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lines != nil {
		t.Fatalf("expected nil lines for empty file, got %v", lines)
	}
}

func TestMetadataExtractedWithBlock(t *testing.T) {
	lines := strings.Split(`# Regular comment
# stm:group=production
# stm:autoconnect=true
Host prod
    HostName prod.example.com
    User deploy`, "\n")

	block := FindHostBlock(lines, "prod")
	if block == nil {
		t.Fatal("expected to find prod block")
	}
	// The stm comments should be included in the block
	if block.StartLine != 1 {
		t.Errorf("StartLine = %d, want 1 (should include stm comments but not regular comment)", block.StartLine)
	}
}
