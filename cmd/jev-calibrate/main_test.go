package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestCalibrationCLIRequiresExplicitLocalInput(t *testing.T) {
	var out, stderr bytes.Buffer
	if code := run(nil, strings.NewReader(""), &out, &stderr); code == 0 || !strings.Contains(stderr.String(), "input") {
		t.Fatalf("%d %s", code, stderr.String())
	}
}

func TestCalibrationCLIRejectsInvalidJSONAndThreshold(t *testing.T) {
	for _, args := range [][]string{{"--input", "-"}, {"--input", "-", "--thresholds", "nope"}, {"--input", "-", "--thresholds", "1.2"}, {"--input", "-", "--thresholds", "NaN"}, {"--input", "-", "--thresholds", "Inf"}} {
		var out, stderr bytes.Buffer
		if code := run(args, strings.NewReader(`{"unknown":true}`), &out, &stderr); code == 0 {
			t.Fatal("accepted invalid input")
		}
	}
}

func TestCalibrationCLIReportsFixture(t *testing.T) {
	var out, stderr bytes.Buffer
	code := run([]string{"--input", "../../testdata/evidence/calibration.json"}, strings.NewReader(""), &out, &stderr)
	if code != 0 || !strings.Contains(out.String(), "advisory_not_a_calibrated_cutoff") {
		t.Fatalf("%d %s %s", code, out.String(), stderr.String())
	}
}

func TestThresholdParserRejectsNonFinite(t *testing.T) {
	for _, s := range []string{"NaN", "Inf", "-Inf"} {
		if _, err := parseThresholds(s); err == nil {
			t.Fatal("accepted nonfinite threshold", s)
		}
	}
}
