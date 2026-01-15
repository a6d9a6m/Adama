package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type benchmarkRecord struct {
	Scenario       string    `json:"scenario"`
	Environment    string    `json:"environment"`
	RequestsPerSec float64   `json:"requests_per_sec"`
	TransferPerSec string    `json:"transfer_per_sec"`
	Timeouts       int       `json:"timeouts"`
	CreatedAt      time.Time `json:"created_at"`
}

func main() {
	var (
		input       string
		output      string
		scenario    string
		environment string
	)
	flag.StringVar(&input, "input", "", "wrk output file")
	flag.StringVar(&output, "output", filepath.Join("benchmarks", "wrk", "history.jsonl"), "record output file")
	flag.StringVar(&scenario, "scenario", "adama-order", "benchmark scenario")
	flag.StringVar(&environment, "env", "local", "benchmark environment")
	flag.Parse()

	if input == "" {
		fmt.Fprintln(os.Stderr, "-input is required")
		os.Exit(1)
	}

	record, err := parseWRKFile(input, scenario, environment)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := appendRecord(output, record); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseWRKFile(path, scenario, environment string) (benchmarkRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return benchmarkRecord{}, err
	}
	defer file.Close()

	record := benchmarkRecord{
		Scenario:    scenario,
		Environment: environment,
		CreatedAt:   time.Now(),
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
		case strings.Contains(line, "timeout"):
			record.Timeouts = parseTimeouts(line)
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
