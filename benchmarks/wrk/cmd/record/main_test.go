package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseWRKFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wrk.txt")
	content := `Running 5s test @ http://127.0.0.1:8080/api/v1/goods/list
  4 threads and 10 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency     7.22ms   20.48ms 202.93ms   96.16%
    Req/Sec   560.23    135.44   717.00     87.82%
  Latency Distribution
     50%    3.10ms
     90%    5.61ms
     99%  133.76ms
  11077 requests in 5.02s, 1.15MB read
Requests/sec:   2208.23
Transfer/sec:    235.06KB
Socket errors: connect 0, read 0, write 0, timeout 40
Non-2xx responses: 3`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	record, err := parseWRKFile(path, "goods_list", "local", "gateway", "primary", "test")
	if err != nil {
		t.Fatalf("parseWRKFile() error = %v", err)
	}
	if record.RequestsPerSec != 2208.23 {
		t.Fatalf("RequestsPerSec = %v", record.RequestsPerSec)
	}
	if record.Timeouts != 40 {
		t.Fatalf("Timeouts = %d", record.Timeouts)
	}
	if record.Non2xx != 3 {
		t.Fatalf("Non2xx = %d", record.Non2xx)
	}
	if record.AvgLatencyMs != 7.22 {
		t.Fatalf("AvgLatencyMs = %v", record.AvgLatencyMs)
	}
	if record.P50LatencyMs != 3.10 || record.P90LatencyMs != 5.61 || record.P99LatencyMs != 133.76 {
		t.Fatalf("percentiles = %#v", record)
	}
}

func TestParseWRKFile_SecondsAndMicros(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wrk.txt")
	content := `Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency     1.50s    2.00ms   2.00s    80.00%
  Latency Distribution
     50%    900us
     90%    1.50ms
     99%    2.00s
Requests/sec:   10.00`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	record, err := parseWRKFile(path, "mixed_latency", "local", "service", "primary", "")
	if err != nil {
		t.Fatalf("parseWRKFile() error = %v", err)
	}
	if record.AvgLatencyMs != 1500 {
		t.Fatalf("AvgLatencyMs = %v", record.AvgLatencyMs)
	}
	if record.P50LatencyMs != 0.9 || record.P90LatencyMs != 1.5 || record.P99LatencyMs != 2000 {
		t.Fatalf("percentiles = %#v", record)
	}
}

func TestParseWRKFile_MissingRequestsPerSec(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wrk.txt")
	if err := os.WriteFile(path, []byte("Latency     7.22ms"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := parseWRKFile(path, "invalid", "local", "gateway", "primary", ""); err == nil {
		t.Fatal("expected error for missing Requests/sec")
	}
}
