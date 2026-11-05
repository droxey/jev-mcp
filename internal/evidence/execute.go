package evidence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"maps"
	"math"
	"sync"
	"time"
)

const (
	dependentQuestions    = 2
	reasonBudgetExhausted = "budget_exhausted"
	reasonCancelled       = "cancelled"
	defaultInputBytes     = 4 << 20
	maxInputBytes         = 64 << 20
	defaultTimeout        = 30 * time.Second
	maxTimeout            = 2 * time.Minute
	defaultCalls          = 512
	maxCalls              = 1024
	defaultBatchSize      = 8
	maxBatchSize          = 16
	defaultConcurrency    = 2
	maxConcurrency        = 16
	maxRetries            = 2
	maxAnswerBytes        = 64 << 10
	probabilityTolerance  = 0.000001
)

// Evaluator executes a bounded, dependent sequence of documentary questions.
// A Provider must honor context cancellation; no network or file retrieval occurs here.
type Evaluator struct {
	provider Provider
	limits   Limits
}

// NewEvaluator supplies conservative local limits. Zero fields use defaults;
// out-of-range positive settings are clamped to hard ceilings.
func NewEvaluator(p Provider, l Limits) *Evaluator {
	if l.Timeout <= 0 {
		l.Timeout = defaultTimeout
	}
	l.Timeout = min(l.Timeout, maxTimeout)
	l.MaxCalls = bounded(l.MaxCalls, defaultCalls, maxCalls)
	l.BatchSize = bounded(l.BatchSize, defaultBatchSize, maxBatchSize)
	l.Concurrency = bounded(l.Concurrency, defaultConcurrency, maxConcurrency)
	l.MaxInputBytes = bounded(l.MaxInputBytes, defaultInputBytes, maxInputBytes)
	l.MaxRetries = max(0, min(l.MaxRetries, maxRetries))
	return &Evaluator{provider: p, limits: l}
}

func bounded(value, fallback, ceiling int) int {
	if value <= 0 {
		return fallback
	}
	return min(value, ceiling)
}

type execution struct {
	e            *Evaluator
	mu           sync.Mutex
	calls, input int
}

func (x *execution) reserve(calls []Call) bool {
	n := 0
	for _, c := range calls {
		b, err := json.Marshal(c)
		if err != nil || len(c.State) == 0 {
			return false
		}
		n += len(b)
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.calls+len(calls) > x.e.limits.MaxCalls || x.input+n > x.e.limits.MaxInputBytes {
		return false
	}
	x.calls += len(calls)
	x.input += n
	return true
}

func failure(c Call, code string) Answer {
	validity := ValidityFailure
	status := StatusFailed
	if code == reasonBudgetExhausted || code == reasonCancelled {
		validity = ValidityIncomplete
		status = StatusSkipped
	}
	if code == OutcomeInvalidProvider {
		validity = ValidityInvalid
	}
	return Answer{ID: c.ID, Status: status, Validity: validity, Error: code}
}

func (x *execution) run(ctx context.Context, calls []Call) map[string]Answer {
	out := make(map[string]Answer, len(calls))
	if len(calls) == 0 {
		return out
	}
	jobs := make(chan []Call)
	var mu sync.Mutex
	var wg sync.WaitGroup
	workers := min(x.e.limits.Concurrency, (len(calls)+x.e.limits.BatchSize-1)/x.e.limits.BatchSize)
	for range workers {
		wg.Go(func() {
			for batch := range jobs {
				answers := x.batch(ctx, batch)
				mu.Lock()
				for id := range answers {
					out[id] = answers[id]
				}
				mu.Unlock()
			}
		})
	}
	for start := 0; start < len(calls); start += x.e.limits.BatchSize {
		end := min(start+x.e.limits.BatchSize, len(calls))
		jobs <- calls[start:end]
	}
	close(jobs)
	wg.Wait()
	return out
}

func (x *execution) batch(ctx context.Context, calls []Call) map[string]Answer {
	answers, code := x.request(ctx, calls)
	if code != "" {
		return failedBatch(calls, code)
	}
	out := make(map[string]Answer, len(calls))
	expected := map[string]Call{}
	for _, c := range calls {
		expected[c.ID] = c
	}
	for i := range answers {
		a := &answers[i]
		c, ok := expected[a.ID]
		if !ok {
			return failedBatch(calls, OutcomeInvalidProvider)
		}
		if _, duplicate := out[a.ID]; duplicate {
			return failedBatch(calls, OutcomeInvalidProvider)
		}
		out[a.ID] = validateAnswer(c, a)
	}
	for _, c := range calls {
		if _, ok := out[c.ID]; !ok {
			out[c.ID] = failure(c, OutcomeInvalidProvider)
		}
	}
	return out
}

func failedBatch(calls []Call, code string) map[string]Answer {
	out := make(map[string]Answer, len(calls))
	for _, c := range calls {
		out[c.ID] = failure(c, code)
	}
	return out
}

func (x *execution) request(ctx context.Context, calls []Call) (answers []Answer, reason string) {
	if ctx.Err() != nil {
		return nil, reasonCancelled
	}
	if x.e.provider == nil {
		return nil, OutcomeProviderFailure
	}
	for attempt := 0; attempt <= x.e.limits.MaxRetries; attempt++ {
		if ctx.Err() != nil {
			return nil, reasonCancelled
		}
		if !x.reserve(calls) {
			return nil, reasonBudgetExhausted
		}
		answers, err := x.e.provider.Evaluate(ctx, calls)
		if ctx.Err() != nil {
			return nil, reasonCancelled
		}
		if err == nil {
			return answers, ""
		}
		retry, ok := errors.AsType[interface {
			error
			Retryable() bool
		}](err)
		if !ok || !retry.Retryable() {
			break
		}
	}
	return nil, OutcomeProviderFailure
}

func invalidAnswer(a *Answer) Answer {
	a.Status = StatusFailed
	a.Validity = ValidityInvalid
	a.Error = OutcomeInvalidProvider
	a.SemanticLevel = nil
	sanitizeRejected(a)
	return *a
}

func validateAnswer(c Call, a *Answer) Answer {
	if a.Status != StatusSuccess {
		if a.Status != StatusFailed && a.Status != StatusSkipped {
			return invalidAnswer(a)
		}
		if a.Error == OutcomeInvalidProvider || a.Validity == ValidityInvalid {
			return invalidAnswer(a)
		}
		a.SemanticLevel = nil
		if a.Status == StatusSkipped {
			a.Validity = ValidityIncomplete
			a.Error = "skipped"
			sanitizeRejected(a)
			return *a
		}
		a.Validity = ValidityFailure
		a.Error = OutcomeProviderFailure
		sanitizeRejected(a)
		return *a
	}
	if !validSuccess(c, a) {
		return invalidAnswer(a)
	}
	a.Error = ""
	return *a
}

func validSuccess(c Call, a *Answer) bool {
	if !validNumbers(a) || len(a.Raw) > maxAnswerBytes || (len(a.Raw) > 0 && !json.Valid(a.Raw)) {
		return false
	}
	if c.Stage == StageSufficiency {
		if a.SemanticLevel != nil && (*a.SemanticLevel < 0 || *a.SemanticLevel > supportLevelComplete) {
			return false
		}
		if a.Score != nil && (*a.Score < 0 || *a.Score > supportLevelComplete) {
			return false
		}
		a.Validity = ValiditySuccess
	} else {
		if _, ok := c.Options[a.Selected]; !ok {
			return false
		}
		a.Validity = ValiditySuccess
	}
	for k := range a.Probabilities {
		if _, ok := c.Options[k]; !ok {
			return false
		}
	}
	return true
}

func validNumbers(a *Answer) bool {
	for _, p := range []*float64{a.Probability, a.Confidence} {
		if p != nil && (!finite(*p) || *p < 0 || *p > 1) {
			return false
		}
	}
	if a.Score != nil && !finite(*a.Score) {
		return false
	}
	sum := 0.0
	for _, p := range a.Probabilities {
		if !finite(p) || p < 0 || p > 1 {
			return false
		}
		sum += p
	}
	return len(a.Probabilities) == 0 || math.Abs(sum-1) < probabilityTolerance
}
func finite(n float64) bool { return !math.IsNaN(n) && !math.IsInf(n, 0) }

func sanitizeRejected(a *Answer) {
	if a.Score != nil && !finite(*a.Score) {
		a.Score = nil
	}
	if a.Probability != nil && !finite(*a.Probability) {
		a.Probability = nil
	}
	if a.Confidence != nil && !finite(*a.Confidence) {
		a.Confidence = nil
	}
	a.Probabilities = maps.Clone(a.Probabilities)
	for k, v := range a.Probabilities {
		if !finite(v) {
			delete(a.Probabilities, k)
		}
	}
	if len(a.Raw) > maxAnswerBytes {
		h := sha256.Sum256(a.Raw)
		a.RawSHA256 = hex.EncodeToString(h[:])
		a.Raw = nil
		a.RawEncoding = "omitted_size_limit"
		return
	}
	if len(a.Raw) > 0 && !json.Valid(a.Raw) {
		h := sha256.Sum256(a.Raw)
		a.RawSHA256 = hex.EncodeToString(h[:])
		b, err := json.Marshal([]byte(a.Raw))
		if err == nil {
			a.Raw = b
			a.RawEncoding = "base64"
		} else {
			a.Raw = nil
		}
	}
}
