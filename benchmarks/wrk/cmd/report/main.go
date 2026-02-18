package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
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

func main() {
	var input string
	var scenario string
	flag.StringVar(&input, "input", "benchmarks/wrk/results/history.jsonl", "benchmark history file")
	flag.StringVar(&scenario, "scenario", "", "scenario filter")
	flag.Parse()

	records, err := loadRecords(input, scenario)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(records) == 0 {
		fmt.Fprintln(os.Stderr, "no benchmark records matched")
		os.Exit(1)
	}

	latest := records[len(records)-1]
	best := latest
	for _, record := range records {
		if record.RequestsPerSec > best.RequestsPerSec {
			best = record
		}
	}

	fmt.Printf("scenario: %s\n", latest.Scenario)
	fmt.Printf("records: %d\n", len(records))
	fmt.Printf("latest: %.2f req/s, avg %.2f ms, p99 %.2f ms, non2xx %d, entrypoint %s, db %s\n",
		latest.RequestsPerSec, latest.AvgLatencyMs, latest.P99LatencyMs, latest.Non2xx, latest.EntryPoint, latest.DatabaseMode)
	fmt.Printf("best: %.2f req/s, avg %.2f ms, p99 %.2f ms, non2xx %d, entrypoint %s, db %s\n",
		best.RequestsPerSec, best.AvgLatencyMs, best.P99LatencyMs, best.Non2xx, best.EntryPoint, best.DatabaseMode)
}

func loadRecords(path, scenario string) ([]benchmarkRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	records := make([]benchmarkRecord, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var record benchmarkRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, err
		}
		if scenario != "" && record.Scenario != scenario {
			continue
		}
		records = append(records, record)
	}
	return records, scanner.Err()
}
