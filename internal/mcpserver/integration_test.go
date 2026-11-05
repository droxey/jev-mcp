package mcpserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tphakala/jev-mcp/internal/evidence"
	"github.com/tphakala/jev-mcp/internal/typesafe"
)

func TestPreparationMCPProviderPolicyAndDependentChoice(t *testing.T) {
	for _, transport := range []string{"http", "stdio"} {
		t.Run(transport, func(t *testing.T) {
			for _, retry := range []bool{false, true} {
				t.Run(map[bool]string{false: "successful", true: "bounded retry"}[retry], func(t *testing.T) {
					harness, client := newTypeSafeHarness(t, retry)
					limits := evidence.Limits{Timeout: 5 * time.Second, MaxCalls: 5, BatchSize: 8, Concurrency: 1, MaxRetries: 1}
					deps := &Deps{Provider: client, Model: "jev-fixture-v1", Limits: limits}
					ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
					defer cancel()
					session := evaluationSession(ctx, t, deps, transport)
					result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "evaluate_plan", Arguments: integrationPlan()})
					if err != nil || result.IsError {
						t.Fatalf("MCP evaluation failed: %v", err)
					}
					data, err := json.Marshal(result.StructuredContent)
					if err != nil {
						t.Fatal(err)
					}
					var report evidence.Report
					if err = json.Unmarshal(data, &report); err != nil {
						t.Fatal(err)
					}
					assertGroundedReport(t, data, &report, retry)
					harness.mu.Lock()
					defer harness.mu.Unlock()
					if len(harness.stages) != 4 || harness.stages[0] != "relation" || harness.stages[len(harness.stages)-1] != "choice" {
						t.Fatalf("stages=%v", harness.stages)
					}
				})
			}
		})
	}
}

type mockQuestion struct {
	Type         string          `json:"type"`
	Instructions string          `json:"instructions"`
	Criteria     json.RawMessage `json:"criteria"`
}

type typeSafeHarness struct {
	mu       sync.Mutex
	stages   []string
	attempts int
	retry    bool
	t        *testing.T
}

func newTypeSafeHarness(t *testing.T, retry bool) (*typeSafeHarness, *typesafe.Client) {
	t.Helper()
	harness := &typeSafeHarness{t: t, retry: retry}
	server := httptest.NewServer(harness)
	t.Cleanup(server.Close)
	client, err := typesafe.New(typesafe.Config{APIKey: "fixture-key", Model: "jev-fixture-v1", Endpoint: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	return harness, client
}

func (h *typeSafeHarness) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.attempts++
	if h.retry && h.attempts == 1 {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("fixture-key"))
		return
	}
	if r.URL.Path != "/v1/systemone" || r.Header.Get("Authorization") != "Bearer fixture-key" {
		h.t.Error("provider contract mismatch")
	}
	var request struct {
		State struct {
			Records map[string]json.RawMessage `json:"records"`
		} `json:"state"`
		Questions map[string]mockQuestion `json:"questions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		h.t.Error(err)
		return
	}
	answers := make(map[string]any, len(request.Questions))
	for id, q := range request.Questions {
		answers[id] = h.answer(q, request.State.Records[id])
	}
	if err := json.NewEncoder(w).Encode(map[string]any{"model": "jev-fixture-v1", "answers": answers}); err != nil {
		h.t.Error(err)
	}
}

func (h *typeSafeHarness) answer(q mockQuestion, state json.RawMessage) any {
	if q.Type == "score" {
		if len(h.stages) == 0 || h.stages[0] != "relation" {
			h.t.Error("sufficiency preceded relations")
		}
		if !strings.Contains(string(state), "supporting_passages") || !strings.Contains(string(state), "passage-export") {
			h.t.Error("missing identified supporting subset")
		}
		var levels []string
		if err := json.Unmarshal(q.Criteria, &levels); err != nil {
			h.t.Error(err)
		}
		if len(levels) != 3 {
			h.t.Error("wrong score rubric")
			return nil
		}
		h.stages = append(h.stages, "sufficiency")
		return map[string]any{"type": "score", "score": 1.7, "confidence": 0.55, "probabilities": map[string]float64{"0": 0.1, "1": 0.1, "2": 0.8}, "legend": map[string]string{"0": levels[0], "1": levels[1], "2": levels[2]}}
	}
	var options map[string]string
	if err := json.Unmarshal(q.Criteria, &options); err != nil {
		h.t.Error(err)
	}
	selected := "candidate-a"
	if _, ok := options[evidence.Supports]; ok {
		selected = evidence.Supports
		h.stages = append(h.stages, "relation")
	} else if _, ok := options["direct_statement"]; ok {
		selected = "direct_statement"
		h.stages = append(h.stages, "directness")
	} else {
		if !slices.Contains(h.stages, "sufficiency") || !slices.Contains(h.stages, "directness") || !strings.Contains(string(state), "eligible_candidates") {
			h.t.Error("choice preceded dependent policy gates")
		}
		h.stages = append(h.stages, "choice")
	}
	probabilities := make(map[string]float64, len(options))
	for key := range options {
		probabilities[key] = 0.3 / float64(len(options)-1)
	}
	probabilities[selected] = 0.7
	return map[string]any{"type": "choice", "choice": selected, "confidence": 0.8, "probabilities": probabilities}
}

func evaluationSession(ctx context.Context, t *testing.T, d *Deps, transport string) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
	if transport == "http" {
		handler, err := HTTPHandler(d, HTTPOptions{AllowUnauthenticatedLoopback: true})
		if err != nil {
			t.Fatal(err)
		}
		server := httptest.NewServer(handler)
		t.Cleanup(server.Close)
		session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: server.URL + "/mcp"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = session.Close() })
		return session
	}
	callCtx, cancel := context.WithCancel(ctx)
	sr, cw := io.Pipe()
	cr, sw := io.Pipe()
	done := make(chan error, 1)
	go func() { done <- ServeIO(callCtx, d, sr, sw) }()
	session, err := client.Connect(ctx, &mcp.IOTransport{Reader: cr, Writer: cw}, nil)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		_ = session.Close()
		if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
			t.Error(err)
		}
	})
	return session
}

func assertGroundedReport(t *testing.T, data []byte, report *evidence.Report, retry bool) {
	t.Helper()
	if report.Outcome != evidence.OutcomeSelected || report.SelectedID != "candidate-a" || len(report.Claims) != 1 || !report.Claims[0].Complete || report.PlanSHA256 == "" {
		t.Fatalf("report %+v", report)
	}
	wantCalls := 4
	if retry {
		wantCalls++
	}
	if report.Calls != wantCalls {
		t.Fatalf("calls=%d want%d", report.Calls, wantCalls)
	}
	if len(report.Citations) != 1 || !report.Citations[0].Accepted || report.Citations[0].PassageID != "passage-export" || report.Citations[0].SourceSHA256 == "" {
		t.Fatalf("citations=%+v", report.Citations)
	}
	assertScoreUncertainty(t, report.Claims[0].Answer)
	assertChoiceUncertainty(t, report.Choice)

	if strings.Contains(string(data), "fixture-key") {
		t.Fatal("provider error disclosed credential")
	}
}

func assertScoreUncertainty(t *testing.T, answer *evidence.Answer) {
	t.Helper()
	if answer == nil || answer.Score == nil || *answer.Score != 1.7 || answer.Confidence == nil || *answer.Confidence != 0.55 || len(answer.Raw) == 0 {
		t.Fatal("raw score uncertainty lost")
	}
}

func assertChoiceUncertainty(t *testing.T, answer *evidence.Answer) {
	t.Helper()
	if answer == nil || answer.Probability == nil || *answer.Probability != 0.7 || answer.Confidence == nil || *answer.Confidence != 0.8 {
		t.Fatal("choice probability and confidence conflated")
	}
}

func integrationPlan() evidence.Plan {
	const text = "Archive includes an offline export command."
	hash := sha256.Sum256([]byte(text))
	profile := evidence.Profile{Version: "evidence-profile-v1", Name: "software-capability", Facets: []string{"provenance", "snapshot"}}
	return evidence.Plan{Version: evidence.SchemaVersion, ID: "integration-plan", Rubric: evidence.RubricVersion, AsOf: "2026-10-21", Profile: profile, Requirements: []evidence.Requirement{{ID: "require-export", Text: "Offline export", Mandatory: true}}, Preferences: []evidence.Preference{{ID: "prefer-archive", Text: "Prefer the archive option", Weight: 1}}, Candidates: []evidence.Candidate{{ID: "candidate-a", Name: "Archive", RequirementClaims: map[string][]string{"require-export": {"claim-export"}}}}, Claims: []evidence.Claim{{ID: "claim-export", Text: "Archive includes offline export.", PassageIDs: []string{"passage-export"}}}, Sources: []evidence.Source{{ID: "source-manual", Title: "Original fictional manual", Origin: "manual-original", Snapshot: text, SHA256: hex.EncodeToString(hash[:]), CapturedAt: "2026-10-21T12:00:00Z", EvidenceKind: "assertion"}}, Passages: []evidence.Passage{{ID: "passage-export", SourceID: "source-manual", StartByte: 0, EndByte: len(text), Excerpt: text}}}
}
