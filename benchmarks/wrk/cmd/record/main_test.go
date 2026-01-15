package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseWRKFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wrk.txt")
	content := `Requests/sec:    2208.23
Transfer/sec:    235.06KB
Socket errors: connect 0, read 0, write 0, timeout 40`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	record, err := parseWRKFile(path, "seckill", "local")
	if err != nil {
		t.Fatalf("parseWRKFile() error = %v", err)
	}
	if record.RequestsPerSec != 2208.23 {
		t.Fatalf("RequestsPerSec = %v", record.RequestsPerSec)
	}
	if record.Timeouts != 40 {
		t.Fatalf("Timeouts = %d", record.Timeouts)
	}
}
