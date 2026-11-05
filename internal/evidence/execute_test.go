package evidence

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type temporaryFixtureError struct{}

func (temporaryFixtureError) Error() string   { return "temporary fixture failure" }
func (temporaryFixtureError) Retryable() bool { return true }

type retryProvider struct {
	attempts atomic.Int32
	inner    fakeProvider
}

func (p *retryProvider) Evaluate(ctx context.Context, c []Call) ([]Answer, error) {
	if p.attempts.Add(1) == 1 {
		return nil, temporaryFixtureError{}
	}
	return p.inner.Evaluate(ctx, c)
}

func TestRetriesConsumeSamePrimitiveBudget(t *testing.T) {
	for _, callCap := range []int{4, 5} {
		p := &retryProvider{}
		r, err := NewEvaluator(p, Limits{MaxRetries: 1, MaxCalls: callCap}).Evaluate(t.Context(), validPlan())
		if err != nil {
			t.Fatal(err)
		}
		want := OutcomeSelected
		if callCap == 4 {
			want = OutcomeIncomplete
		}
		if r.Outcome != want || r.Calls > callCap {
			t.Fatalf("cap=%d calls=%d outcome=%s", callCap, r.Calls, r.Outcome)
		}
	}
}

type gatedProvider struct {
	active, max atomic.Int32
	entered     chan struct{}
	release     chan struct{}
	inner       fakeProvider
}

func (p *gatedProvider) Evaluate(ctx context.Context, c []Call) ([]Answer, error) {
	n := p.active.Add(1)
	defer p.active.Add(-1)
	for {
		old := p.max.Load()
		if n <= old || p.max.CompareAndSwap(old, n) {
			break
		}
	}
	select {
	case p.entered <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case <-p.release:
		return p.inner.Evaluate(ctx, c)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestIndependentBatchesRespectConcurrencyAndCancellation(t *testing.T) {
	p := validPlan()
	addEvidence(&p, "second", "other", "Export works offline.")
	provider := &gatedProvider{entered: make(chan struct{}, 16), release: make(chan struct{})}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan Report, 1)
	go func() {
		r, err := NewEvaluator(provider, Limits{Concurrency: 2, BatchSize: 1}).Evaluate(ctx, p)
		if err != nil {
			t.Error(err)
		}
		done <- r
	}()
	for range 2 {
		select {
		case <-provider.entered:
		case <-time.After(time.Second):
			cancel()
			t.Fatal("independent relation batches did not run concurrently")
		}
	}
	cancel()
	select {
	case r := <-done:
		if r.Outcome != OutcomeIncomplete || provider.max.Load() != 2 || provider.active.Load() != 0 {
			t.Fatalf("outcome=%s max=%d active=%d", r.Outcome, provider.max.Load(), provider.active.Load())
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation failed to release provider calls")
	}
}

type malformedBatchProvider struct {
	kind  string
	inner fakeProvider
}

func (p *malformedBatchProvider) Evaluate(ctx context.Context, c []Call) ([]Answer, error) {
	a, err := p.inner.Evaluate(ctx, c)
	if err != nil {
		return nil, err
	}
	switch p.kind {
	case "missing":
		return nil, nil
	case "duplicate":
		return append(a, a[0]), nil
	case "unknown":
		a[0].ID = "unrequested"
	case "failure":
		return nil, errors.New("sensitive provider details")
	}
	return a, nil
}

func TestBatchIdentityValidationAndProviderFailure(t *testing.T) {
	for _, kind := range []string{"missing", "duplicate", "unknown", "failure"} {
		p := &malformedBatchProvider{kind: kind}
		r, err := NewEvaluator(p, Limits{}).Evaluate(t.Context(), validPlan())
		want := OutcomeInvalidProvider
		if kind == "failure" {
			want = OutcomeProviderFailure
		}
		if err != nil || r.Outcome != want || r.Claims[0].Level != nil {
			t.Fatalf("kind=%s outcome=%s err=%v", kind, r.Outcome, err)
		}
	}
}
