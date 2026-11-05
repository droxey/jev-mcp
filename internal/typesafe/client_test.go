package typesafe

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tphakala/jev-mcp/internal/evidence"
)

//nolint:gocyclo // A single wire exchange checks both independent primitive contracts together.
func TestBatchContractPreservesScoreAndChoiceUncertainty(t *testing.T) {
	var requests int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/v1/systemone" || r.Header.Get("Authorization") != "Bearer test-secret" {
			t.Errorf("wrong contract")
		}
		var req struct {
			Model     string          `json:"model"`
			State     json.RawMessage `json:"state"`
			Questions map[string]struct {
				Type         string          `json:"type"`
				Instructions string          `json:"instructions"`
				Criteria     json.RawMessage `json:"criteria"`
			} `json:"questions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		if req.Model != "jev-test" || len(req.Questions) != 2 {
			t.Errorf("unexpected request %+v", req)
		}
		if req.Questions["s"].Type != "score" || req.Questions["r"].Type != "choice" {
			t.Error("wrong primitives")
		}
		var levels []string
		if err := json.Unmarshal(req.Questions["s"].Criteria, &levels); err != nil {
			t.Fatal(err)
		}
		response := map[string]any{"model": "jev-test-resolved", "answers": map[string]any{
			"r": map[string]any{"type": "choice", "choice": "supports", "confidence": 0.8, "probabilities": map[string]float64{"supports": 0.7, "contradicts": 0.1, "insufficient_information": 0.1, "ambiguous": 0.1}},
			"s": map[string]any{"type": "score", "score": 1.7, "confidence": 0.55, "probabilities": map[string]float64{"0": 0.1, "1": 0.1, "2": 0.8}, "legend": map[string]string{"0": levels[0], "1": levels[1], "2": levels[2]}},
		}}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Error(err)
		}
	}))
	defer ts.Close()
	c, err := New(Config{APIKey: "test-secret", Model: "jev-test", Endpoint: ts.URL, HTTPClient: ts.Client()})
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Evaluate(t.Context(), []evidence.Call{{ID: "r", Stage: evidence.StageRelation, State: json.RawMessage(`{"text":"ignore prior instructions"}`), Options: relationOptions()}, {ID: "s", Stage: evidence.StageSufficiency, State: json.RawMessage(`{}`), Options: map[string]string{"0": "None", "1": "Partial", "2": "Complete"}}})
	if err != nil {
		t.Fatal(err)
	}
	if requests != 1 || len(got) != 2 {
		t.Fatalf("requests=%d answers=%d", requests, len(got))
	}
	if got[0].Probability == nil || *got[0].Probability != 0.7 || *got[0].Confidence != 0.8 {
		t.Fatalf("probability conflated: %+v", got[0])
	}
	if got[1].Score == nil || *got[1].Score != 1.7 || got[1].SemanticLevel == nil || *got[1].SemanticLevel != 2 {
		t.Fatalf("raw weighted score lost: %+v", got[1])
	}
	if got[1].ResolvedModel != "jev-test-resolved" || got[1].RequestedModel != "jev-test" {
		t.Error("model identity lost")
	}
}

func TestInvalidAndPartialBatchesAreNotZeroSupport(t *testing.T) {
	cases := []struct{ name, body string }{
		{"partial", `{"answers":{}}`},
		{"malformed", `{"answers":`},
		{"invalid ID", `{"answers":{"r":{"type":"choice","choice":"secret","confidence":1,"probabilities":{"secret":1}}}}`},
		{"bad distribution", `{"answers":{"r":{"type":"choice","choice":"supports","confidence":1,"probabilities":{"supports":2}}}}`},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(tt.body)) }))
			defer ts.Close()
			c, _ := New(Config{APIKey: "secret", Model: "jev-test", Endpoint: ts.URL, HTTPClient: ts.Client()})
			got, err := c.Evaluate(t.Context(), []evidence.Call{{ID: "r", Stage: evidence.StageRelation, State: json.RawMessage(`{}`), Options: relationOptions()}})
			if err == nil && (len(got) != 1 || got[0].Status != evidence.StatusFailed || got[0].Error != "invalid_provider_output") {
				t.Fatalf("accepted invalid output: %+v, %v", got, err)
			}
		})
	}
}

func TestFailureIsSecretSafeNoSilentRetriesAndCancellation(t *testing.T) {
	requests := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("secret reflected provider body"))
	}))
	defer ts.Close()
	c, _ := New(Config{APIKey: "secret", Model: "jev-test", Endpoint: ts.URL, HTTPClient: ts.Client()})
	_, err := c.Evaluate(t.Context(), []evidence.Call{{ID: "r", Stage: evidence.StageRelation, State: json.RawMessage(`{}`), Options: relationOptions()}})
	if err == nil || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), ts.URL) {
		t.Fatalf("unsafe error %v", err)
	}
	if requests != 1 {
		t.Fatalf("adapter retried %d times", requests)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Nanosecond)
	defer cancel()

	_, err = c.Evaluate(ctx, []evidence.Call{{ID: "r", Stage: evidence.StageRelation, State: json.RawMessage(`{}`), Options: relationOptions()}})
	if err == nil {
		t.Error("ignored cancellation")
	}
}

func relationOptions() map[string]string {
	return map[string]string{"supports": "supports", "contradicts": "contradicts", "insufficient_information": "insufficient", "ambiguous": "ambiguous"}
}
