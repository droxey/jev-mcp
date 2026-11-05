package mcpserver

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/tphakala/jev-mcp/internal/evidence"
)

type countedProvider struct{ calls atomic.Int32 }

func (p *countedProvider) Evaluate(context.Context, []evidence.Call) ([]evidence.Answer, error) {
	p.calls.Add(1)
	return nil, nil
}

func TestQueuedCancellationRetainsAuditAndDoesNotConsumeSlot(t *testing.T) {
	slots := make(chan struct{}, maxActiveEvaluations)
	for range cap(slots) {
		slots <- struct{}{}
	}
	provider := &countedProvider{}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	plan := integrationPlan()
	_, report, err := evaluatePlan(ctx, &Deps{Provider: provider}, slots, nil, &plan)
	if err != nil {
		t.Fatal(err)
	}
	if report.Outcome != evidence.OutcomeIncomplete || report.Validity != evidence.ValidityIncomplete {
		t.Fatalf("queued cancellation reported as %q/%q", report.Outcome, report.Validity)
	}
	if report.PlanSHA256 == "" || len(report.Claims) != 1 || len(report.Sources) != 1 {
		t.Fatalf("queued cancellation lost audit records: %+v", report)
	}
	if provider.calls.Load() != 0 || report.Calls != 0 || len(slots) != cap(slots) {
		t.Fatalf("cancelled queue consumed resources: provider=%d calls=%d slots=%d", provider.calls.Load(), report.Calls, len(slots))
	}
}
