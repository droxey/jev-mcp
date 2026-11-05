// Command jev-calibrate summarizes supplied held-out labels and recorded reports.
// It performs no provider calls and does not modify production thresholds.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/tphakala/jev-mcp/internal/evidence"
)

const (
	maxDatasetBytes = 16 << 20
	exitUsage       = 2
)

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("jev-calibrate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	input := fs.String("input", "", "local labeled report JSON path, or - for stdin (required)")
	thresholds := fs.String("thresholds", "0.5,0.75,0.9,0.95", "diagnostic complete-support probability thresholds")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if *input == "" || fs.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, "jev-calibrate: --input is required; only local supplied reports are read")
		return exitUsage
	}
	values, err := parseThresholds(*thresholds)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitUsage
	}
	reader := stdin
	if *input != "-" {
		f, openErr := os.Open(*input)
		if openErr != nil {
			_, _ = fmt.Fprintln(stderr, "jev-calibrate: cannot open local input")
			return 1
		}
		defer func() { _ = f.Close() }()
		reader = f
	}
	data, err := readDataset(reader)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	report, err := evidence.Calibrate(data, values)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		_, _ = fmt.Fprintln(stderr, "jev-calibrate: cannot write report")
		return 1
	}
	return 0
}

func parseThresholds(raw string) ([]float64, error) {
	values := []float64{}
	for part := range strings.SplitSeq(raw, ",") {
		v, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1 {
			return nil, errors.New("jev-calibrate: thresholds must be numbers in [0,1]")
		}
		values = append(values, v)
	}
	return values, nil
}

func readDataset(r io.Reader) (evidence.CalibrationSet, error) {
	b, err := io.ReadAll(io.LimitReader(r, maxDatasetBytes+1))
	if err != nil {
		return evidence.CalibrationSet{}, errors.New("jev-calibrate: cannot read input")
	}
	if len(b) > maxDatasetBytes {
		return evidence.CalibrationSet{}, errors.New("jev-calibrate: dataset exceeds 16 MiB")
	}
	var set evidence.CalibrationSet
	decoder := json.NewDecoder(strings.NewReader(string(b)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&set); err != nil {
		return set, errors.New("jev-calibrate: invalid labeled report JSON")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return set, errors.New("jev-calibrate: expected exactly one JSON value")
	}
	return set, nil
}
