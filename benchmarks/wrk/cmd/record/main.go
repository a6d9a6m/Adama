package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type socketErrors struct {
	Connect int `json:"connect"`
	Read    int `json:"read"`
	Write   int `json:"write"`
	Timeout int `json:"timeout"`
}

type benchmarkRecord struct {
	Scenario       string       `json:"scenario"`
	Environment    string       `json:"environment"`
	EntryPoint     string       `json:"entrypoint"`
	DatabaseMode   string       `json:"database_mode"`
	BuildRef       string       `json:"build_ref"`
	RequestsPerSec float64      `json:"requests_per_sec"`
	TransferPerSec string       `json:"transfer_per_sec"`
	AvgLatencyMs   float64      `json:"avg_latency_ms"`
	P50LatencyMs   float64      `json:"p50_latency_ms"`
	P90LatencyMs   float64      `json:"p90_latency_ms"`
	P99LatencyMs   float64      `json:"p99_latency_ms"`
	Timeouts       int          `json:"timeouts"`
	Non2xx         int          `json:"non_2xx"`
	SocketErrors   socketErrors `json:"socket_errors"`
	InputFile      string       `json:"input_file"`
	CreatedAt      time.Time    `json:"created_at"`
}

var (
	latencyLinePattern = regexp.MustCompile(`^Latency\s+([0-9.]+[a-zA-Z]+)`)
	percentilePattern  = regexp.MustCompile(`^(50|90|99)%\s+([0-9.]+[a-zA-Z]+)$`)
	non2xxPattern      = regexp.MustCompile(`^Non-2xx responses:\s+(\d+)$`)
	socketErrorPattern = regexp.MustCompile(`^Socket errors:\s+connect\s+(\d+),\s+read\s+(\d+),\s+write\s+(\d+),\s+timeout\s+(\d+)$`)
)

func main() {
	var (
		input        string
		output       string
		scenario     string
		environment  string
		entrypoint   string
		databaseMode string
		buildRef     string
	)
	flag.StringVar(&input, "input", "", "wrk output file")
	flag.StringVar(&output, "output", filepath.Join("benchmarks", "wrk", "results", "history.jsonl"), "record output file")
	flag.StringVar(&scenario, "scenario", "benchmark", "benchmark scenario")
	flag.StringVar(&environment, "env", "local", "benchmark environment")
	flag.StringVar(&entrypoint, "entrypoint", "gateway", "benchmark entrypoint")
	flag.StringVar(&databaseMode, "db-mode", "primary", "database mode")
	flag.StringVar(&buildRef, "build-ref", "", "build or commit reference")
	flag.Parse()

	if input == "" {
		fmt.Fprintln(os.Stderr, "-input is required")
		os.Exit(1)
	}

	record, err := parseWRKFile(input, scenario, environment, entrypoint, databaseMode, buildRef)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := appendRecord(output, record); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseWRKFile(path, scenario, environment, entrypoint, databaseMode, buildRef string) (benchmarkRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return benchmarkRecord{}, err
	}
	defer file.Close()

	record := benchmarkRecord{
		Scenario:     scenario,
		Environment:  environment,
		EntryPoint:   entrypoint,
		DatabaseMode: databaseMode,
		BuildRef:     buildRef,
		InputFile:    filepath.ToSlash(path),
		CreatedAt:    time.Now(),
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case strings.HasPrefix(line, "Requests/sec:"):
			value := strings.TrimSpace(strings.TrimPrefix(line, "Requests/sec:"))
			record.RequestsPerSec, err = strconv.ParseFloat(value, 64)
			if err != nil {
				return benchmarkRecord{}, err
			}
		case strings.HasPrefix(line, "Transfer/sec:"):
			record.TransferPerSec = strings.TrimSpace(strings.TrimPrefix(line, "Transfer/sec:"))
		case latencyLinePattern.MatchString(line):
			matches := latencyLinePattern.FindStringSubmatch(line)
			record.AvgLatencyMs, err = parseLatencyMs(matches[1])
			if err != nil {
				return benchmarkRecord{}, err
			}
		case percentilePattern.MatchString(line):
			matches := percentilePattern.FindStringSubmatch(line)
			value, parseErr := parseLatencyMs(matches[2])
			if parseErr != nil {
				return benchmarkRecord{}, parseErr
			}
			switch matches[1] {
			case "50":
				record.P50LatencyMs = value
			case "90":
				record.P90LatencyMs = value
			case "99":
				record.P99LatencyMs = value
			}
		case non2xxPattern.MatchString(line):
			matches := non2xxPattern.FindStringSubmatch(line)
			record.Non2xx, _ = strconv.Atoi(matches[1])
		case socketErrorPattern.MatchString(line):
			matches := socketErrorPattern.FindStringSubmatch(line)
			record.SocketErrors.Connect, _ = strconv.Atoi(matches[1])
			record.SocketErrors.Read, _ = strconv.Atoi(matches[2])
			record.SocketErrors.Write, _ = strconv.Atoi(matches[3])
			record.SocketErrors.Timeout, _ = strconv.Atoi(matches[4])
			record.Timeouts = record.SocketErrors.Timeout
		case strings.Contains(line, "timeout"):
			record.Timeouts = parseTimeouts(line)
			record.SocketErrors.Timeout = record.Timeouts
		}
	}
	if err := scanner.Err(); err != nil {
		return benchmarkRecord{}, err
	}
	if record.RequestsPerSec == 0 {
		return benchmarkRecord{}, fmt.Errorf("invalid wrk output: missing Requests/sec")
	}
	return record, nil
}

func parseLatencyMs(raw string) (float64, error) {
	switch {
	case strings.HasSuffix(raw, "us"):
		value, err := strconv.ParseFloat(strings.TrimSuffix(raw, "us"), 64)
		if err != nil {
			return 0, err
		}
		return value / 1000, nil
	case strings.HasSuffix(raw, "ms"):
		return strconv.ParseFloat(strings.TrimSuffix(raw, "ms"), 64)
	case strings.HasSuffix(raw, "s"):
		value, err := strconv.ParseFloat(strings.TrimSuffix(raw, "s"), 64)
		if err != nil {
			return 0, err
		}
		return value * 1000, nil
	default:
		return 0, fmt.Errorf("unsupported latency value: %s", raw)
	}
}

func parseTimeouts(line string) int {
	fields := strings.Fields(strings.ReplaceAll(line, ",", ""))
	for i := 0; i < len(fields)-1; i++ {
		if fields[i] == "timeout" {
			value, _ := strconv.Atoi(fields[i+1])
			return value
		}
	}
	return 0
}

func appendRecord(path string, record benchmarkRecord) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	return encoder.Encode(record)
}
