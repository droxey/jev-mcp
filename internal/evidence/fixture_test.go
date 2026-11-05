package evidence

import (
	"bytes"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
)

func fixtureProvider() *fakeProvider {
	return &fakeProvider{mutate: func(c Call, a *Answer) {
		if c.Stage == StageSufficiency && a.SemanticLevel != nil {
			a.Probabilities = map[string]float64{"0": 0, "1": 0, "2": 0}
			a.Probabilities[strconv.Itoa(*a.SemanticLevel)] = 1
			score := float64(*a.SemanticLevel)
			a.Score = &score
		}
	}}
}

func TestFixturePlanAndReportStayReproducible(t *testing.T) {
	b, err := os.ReadFile("../../testdata/evidence/plan.json")
	if err != nil {
		t.Fatal(err)
	}
	p, err := DecodePlan(strings.NewReader(string(b)))
	if err != nil {
		t.Fatal(err)
	}
	expected, err := os.ReadFile("../../testdata/evidence/expected-report.json")
	if err != nil {
		t.Fatal(err)
	}
	var want Report
	if err := json.Unmarshal(expected, &want); err != nil {
		t.Fatal(err)
	}
	got := evaluate(t, p, fixtureProvider())
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotJSON, wantJSON) {
		t.Fatal("fixture report differs from deterministic local provider output")
	}
}
