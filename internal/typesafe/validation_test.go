package typesafe

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tphakala/jev-mcp/internal/evidence"
)

func TestScoreMappingRequiresLegendAndPreservesTies(t *testing.T) {
	options := map[string]string{"0": "None", "1": "Partial", "2": "Complete"}
	for _, tt := range []struct {
		name   string
		probs  map[string]float64
		legend map[string]string
		score  float64
		valid  bool
		level  *int
	}{
		{"tied mean is not partial", map[string]float64{"0": 0.5, "1": 0, "2": 0.5}, options, 1, true, nil},
		{"wrong legend", map[string]float64{"0": 0, "1": 0, "2": 1}, map[string]string{"0": "Complete", "1": "Partial", "2": "None"}, 2, false, nil},
		{"mean mismatch", map[string]float64{"0": 0, "1": 0, "2": 1}, options, 1, false, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			answer := wireAnswer{Type: "score", Score: &tt.score, Probabilities: tt.probs, Legend: tt.legend}
			level, valid := scoreResult(answer, options)
			if valid != tt.valid || (tt.level == nil && level != nil) {
				t.Fatalf("mapped level=%v valid=%v", level, valid)
			}
		})
	}
}

func TestRedirectDoesNotForwardCredentials(t *testing.T) {
	forwarded := false
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded = true }))
	defer destination.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	c, err := New(Config{APIKey: "fixture-key", Model: "jev-test", Endpoint: redirect.URL, HTTPClient: redirect.Client()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Evaluate(t.Context(), []evidence.Call{{ID: "r", Stage: evidence.StageRelation, State: json.RawMessage(`{}`), Options: relationOptions()}})
	if err == nil || forwarded {
		t.Fatalf("redirect followed=%v error=%v", forwarded, err)
	}
}

func TestDiscoveryNeedsKeyButNoChosenModelAndResponseIsBounded(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer fixture-key" {
			t.Error("discovery contract mismatch")
		}
		_, _ = w.Write([]byte(`{"models":[{"name":"jev-alias"},{"name":"jev-immutable-v1"}]}`))
	}))
	defer ts.Close()
	c, err := New(Config{APIKey: "fixture-key", Endpoint: ts.URL, HTTPClient: ts.Client()})
	if err != nil {
		t.Fatal(err)
	}
	models, err := c.Models(t.Context())
	if err != nil || len(models) != 2 {
		t.Fatalf("discovery failed %v", err)
	}
	if _, err = c.Evaluate(t.Context(), []evidence.Call{{ID: "r"}}); err == nil {
		t.Error("evaluation invented a model")
	}
	c.maxResponse = 4
	if _, err = c.Models(t.Context()); err == nil {
		t.Error("response byte cap ignored")
	}
}

func TestCanceledRequestNeverContactsProvider(t *testing.T) {
	contacted := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { contacted = true; _, _ = w.Write([]byte("fixture")) }))
	defer ts.Close()
	c, _ := New(Config{APIKey: "fixture-key", Model: "jev-test", Endpoint: ts.URL, HTTPClient: ts.Client()})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := c.Evaluate(ctx, []evidence.Call{{ID: "r", Stage: evidence.StageRelation, State: json.RawMessage(`{}`), Options: relationOptions()}})
	if err == nil || contacted || strings.Contains(err.Error(), "fixture-key") {
		t.Fatal("cancellation or secret boundary failed")
	}
}

func TestReflectedCredentialInModelIdentitiesIsNeverReturned(t *testing.T) {
	for _, discovery := range []bool{false, true} {
		t.Run(map[bool]string{false: "evaluation model", true: "discovery model"}[discovery], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				body := `{"model":"jev-fixture-key-reflected","answers":{"r":{"type":"choice","choice":"supports","confidence":1,"probabilities":{"supports":1,"contradicts":0,"insufficient_information":0,"ambiguous":0}}}}`
				if discovery {
					body = `{"models":[{"name":"jev-fixture-key-reflected"}]}`
				}
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			client, err := New(Config{APIKey: "fixture-key", Model: "jev-test", Endpoint: server.URL, HTTPClient: server.Client()})
			if err != nil {
				t.Fatal(err)
			}
			if discovery {
				models, err := client.Models(t.Context())
				if err == nil || len(models) != 0 || strings.Contains(err.Error(), "fixture-key") {
					t.Fatal("discovery accepted or disclosed reflected credential")
				}
				return
			}
			answers, err := client.Evaluate(t.Context(), []evidence.Call{{ID: "r", Stage: evidence.StageRelation, State: json.RawMessage(`{}`), Options: relationOptions()}})
			if err != nil || len(answers) != 1 || answers[0].Status != evidence.StatusFailed || answers[0].Error != evidence.OutcomeInvalidProvider {
				t.Fatal("evaluation accepted credential-bearing model identity")
			}
			encoded, err := json.Marshal(answers)
			if err != nil || strings.Contains(string(encoded), "fixture-key") {
				t.Fatal("evaluation report disclosed reflected credential")
			}
		})
	}
}
