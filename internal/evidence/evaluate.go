package evidence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
)

// ErrInvalidPlan indicates structural or supplied provenance validation failed.
var ErrInvalidPlan = errors.New("evidence: invalid plan")

// Evaluate evaluates explicit snapshots under one deadline. Provider errors are
// report outcomes; malformed plans additionally return ErrInvalidPlan.
//
//nolint:gocritic // Public value API preserves caller ownership; plan payloads reside in slices.
func (e *Evaluator) Evaluate(ctx context.Context, p Plan) (Report, error) {
	r := newReport(&p)
	for _, i := range r.Issues {
		if i.Severity == severityError {
			r.Outcome = "invalid_plan"
			r.Validity = ValidityInvalid
			return r, ErrInvalidPlan
		}
	}
	p = Normalize(p)
	ctx, cancel := context.WithTimeout(ctx, e.limits.Timeout)
	defer cancel()
	x := execution{e: e}
	s := newSession(&p, &r)
	if !s.preflight(e.limits) {
		r.Outcome = OutcomeIncomplete
		r.Validity = ValidityIncomplete
		for i := range r.Claims {
			r.Claims[i].Status = StatusSkipped
			r.Claims[i].Validity = ValidityIncomplete
			r.Claims[i].Reasons = []string{reasonBudgetExhausted}
		}
		s.gateCandidates()
		s.coverage()
		s.audit()
		return r, nil
	}
	relations := x.run(ctx, s.relationCalls())
	s.applyRelations(relations)
	dependent := x.run(ctx, s.dependentCalls())
	s.applyDependent(dependent)
	s.gateClaims()
	s.gateCandidates()
	s.coverage()
	r.Outcome = s.preChoiceOutcome()
	if r.Outcome == "" {
		call := s.choiceCall()
		answers := x.run(ctx, []Call{call})
		a := answers[call.ID]
		r.Choice = &a
		switch a.Validity {
		case ValidityInvalid:
			r.Outcome = OutcomeInvalidProvider
		case ValidityFailure:
			r.Outcome = OutcomeProviderFailure
		case ValidityIncomplete:
			r.Outcome = OutcomeIncomplete
		default:
			r.Outcome = a.Selected
			if a.Selected != OutcomeAbstain && a.Selected != OutcomeInvestigate {
				r.Outcome = OutcomeSelected
				r.SelectedID = a.Selected
			}
		}
	}
	s.audit()
	r.Validity = outcomeValidity(r.Outcome)
	r.Calls = x.calls
	return r, nil
}

func newReport(p *Plan) Report {
	r := Report{Version: SchemaVersion, Rubric: RubricVersion, Policy: PolicyVersion, Calibration: "advisory_uncalibrated", Profile: p.Profile, PlanID: p.ID, Issues: Lint(p), Assumptions: p.Assumptions, Claims: []ClaimResult{}, Candidates: []CandidateResult{}, Quality: []QualityFacet{}}
	for _, issue := range r.Issues {
		if issue.Severity == severityError {
			return r
		}
	}
	b, err := json.Marshal(p)
	if err != nil {
		return r
	}
	hash := sha256.Sum256(b)
	r.PlanSHA256 = hex.EncodeToString(hash[:])
	r.Quality = qualityFacets(p)
	return r
}

type session struct {
	p       *Plan
	r       *Report
	source  map[string]*Source
	passage map[string]Passage
	claim   map[string]*Claim
	result  map[string]*ClaimResult
	origin  map[string]string
}

func newSession(p *Plan, r *Report) *session {
	s := &session{p: p, r: r, source: map[string]*Source{}, passage: map[string]Passage{}, claim: map[string]*Claim{}, result: map[string]*ClaimResult{}, origin: origins(p.Sources)}
	for i := range p.Sources {
		v := &p.Sources[i]
		s.source[v.ID] = v
	}
	for _, v := range p.Passages {
		s.passage[v.ID] = v
	}
	for i := range p.Claims {
		c := &p.Claims[i]
		s.claim[c.ID] = c
		r.Claims = append(r.Claims, ClaimResult{ClaimID: c.ID, Text: c.Text, Scope: c.Scope, Status: StatusSkipped, Validity: ValidityIncomplete, Directness: Unknown, Passages: []PassageResult{}, SupportingPassageIDs: []string{}, ContradictingPassageIDs: []string{}, SupportingOrigins: []string{}, ContradictingOrigins: []string{}})
	}
	for i := range r.Claims {
		s.result[r.Claims[i].ClaimID] = &r.Claims[i]
	}
	return s
}

func call(id, stage string, state any, options map[string]string) Call {
	b, err := json.Marshal(state)
	if err != nil {
		return Call{ID: id, Stage: stage}
	}
	return Call{ID: id, Stage: stage, State: b, Options: options}
}

func (s *session) relationCalls() []Call {
	calls := []Call{}
	for i := range s.p.Claims {
		c := &s.p.Claims[i]
		r := s.result[c.ID]
		for _, id := range c.PassageIDs {
			p := s.passage[id]
			src := s.source[p.SourceID]
			reasons := applicability(c, src, s.p.AsOf, s.p.Profile)
			r.Passages = append(r.Passages, PassageResult{PassageID: id, Applicable: len(reasons) == 0, Origin: s.origin[src.ID], Reasons: reasons})
			if len(reasons) > 0 {
				continue
			}
			calls = append(calls, call("rel:"+c.ID+":"+id, StageRelation, struct {
				Claim   *Claim       `json:"claim"`
				Passage Passage      `json:"passage"`
				Source  SourceRecord `json:"source"`
				Profile Profile      `json:"profile"`
			}{c, p, sourceRecord(src, s.origin[src.ID]), s.p.Profile}, map[string]string{Supports: "Passage supports this exact scoped claim", Contradicts: "Passage contradicts this exact scoped claim", Insufficient: "Passage does not establish either relation", Ambiguous: "Passage relation cannot be resolved"}))
		}
	}
	return calls
}

func (s *session) applyRelations(answers map[string]Answer) {
	for i := range s.p.Claims {
		c := &s.p.Claims[i]
		r := s.result[c.ID]
		for j := range r.Passages {
			p := &r.Passages[j]
			if p.Applicable {
				a := answers["rel:"+c.ID+":"+p.PassageID]
				applyRelation(r, p, &a)
			}
		}
		r.Conflict = len(r.SupportingPassageIDs) > 0 && len(r.ContradictingPassageIDs) > 0
	}
}

func applyRelation(r *ClaimResult, p *PassageResult, a *Answer) {
	p.Answer = a
	if a.Validity != ValiditySuccess {
		mergeFailure(r, a)
		return
	}
	p.Relation = a.Selected
	if r.Status != StatusFailed && !slices.Contains(r.Reasons, "execution_incomplete") {
		r.Status = StatusSuccess
		r.Validity = ValiditySuccess
	}
	switch a.Selected {
	case Supports:
		r.SupportingPassageIDs = append(r.SupportingPassageIDs, p.PassageID)
		r.SupportingOrigins = appendUnique(r.SupportingOrigins, p.Origin)
	case Ambiguous:
		r.Reasons = appendUnique(r.Reasons, "unresolved_evidence_relation")
	case Contradicts:
		r.ContradictingPassageIDs = append(r.ContradictingPassageIDs, p.PassageID)
		r.ContradictingOrigins = appendUnique(r.ContradictingOrigins, p.Origin)
	}
}

func (s *session) dependentCalls() []Call {
	calls := []Call{}
	for i := range s.p.Claims {
		c := &s.p.Claims[i]
		r := s.result[c.ID]
		if len(r.SupportingPassageIDs) == 0 || r.Status != StatusSuccess {
			continue
		}
		// Only the actual supporting subset is sent to dependent sufficiency.
		supporting := make([]Passage, 0, len(r.SupportingPassageIDs))
		for _, id := range r.SupportingPassageIDs {
			supporting = append(supporting, s.passage[id])
		}
		state := struct {
			Claim      *Claim    `json:"claim"`
			Supporting []Passage `json:"supporting_passages"`
			Origins    []string  `json:"supporting_origins"`
			Rubric     string    `json:"rubric"`
		}{c, supporting, r.SupportingOrigins, RubricVersion}
		calls = append(calls, call("suff:"+c.ID, StageSufficiency, state, SupportRubric()), call("direct:"+c.ID, StageDirectness, state, map[string]string{"direct_statement": "Supporting text directly states the claim", "explicit_derivation": "Claim follows through explicit stated derivation", "mixed": "Mixture of direct statement and explicit derivation", Unknown: "Cannot determine directness"}))
	}
	return calls
}

func (s *session) applyDependent(answers map[string]Answer) {
	for i := range s.p.Claims {
		c := &s.p.Claims[i]
		r := s.result[c.ID]
		a, ok := answers["suff:"+c.ID]
		if !ok {
			continue
		}
		d := answers["direct:"+c.ID]
		r.Answer = &a
		r.DirectnessAnswer = &d
		failed := false
		for _, answer := range []*Answer{&a, &d} {
			if answer.Validity != ValiditySuccess {
				mergeFailure(r, answer)
				failed = true
			}
		}
		if failed {
			continue
		}

		r.Level = a.SemanticLevel
		r.Status = StatusSuccess
		r.Validity = a.Validity
		r.Directness = d.Selected
	}
}

func appendUnique(values []string, value string) []string {
	if !slices.Contains(values, value) {
		return append(values, value)
	}
	return values
}

// preflight bounds construction before duplicating any supplied excerpt in calls.
func (s *session) preflight(limits Limits) bool {
	count, input := 1, 0
	for i := range s.p.Claims {
		c := &s.p.Claims[i]
		count += len(c.PassageIDs)
		if len(c.PassageIDs) > 0 {
			count += dependentQuestions
		}
		if count > limits.MaxCalls {
			return false
		}
		supporting := make([]Passage, 0, len(c.PassageIDs))
		for _, id := range c.PassageIDs {
			p := s.passage[id]
			src := s.source[p.SourceID]
			supporting = append(supporting, p)
			b, err := json.Marshal(struct {
				Claim   *Claim       `json:"claim"`
				Passage Passage      `json:"passage"`
				Source  SourceRecord `json:"source"`
			}{c, p, sourceRecord(src, s.origin[src.ID])})
			if err != nil {
				return false
			}
			input += len(b)
			if input > limits.MaxInputBytes {
				return false
			}
		}
		b, err := json.Marshal(struct {
			Claim      *Claim    `json:"claim"`
			Supporting []Passage `json:"supporting_passages"`
		}{c, supporting})
		if err != nil {
			return false
		}
		input += dependentQuestions * len(b)
		if input > limits.MaxInputBytes {
			return false
		}
	}
	return true
}

func outcomeValidity(outcome string) string {
	switch outcome {
	case OutcomeInvalidProvider, "invalid_plan":
		return ValidityInvalid
	case OutcomeProviderFailure:
		return ValidityFailure
	case OutcomeIncomplete:
		return ValidityIncomplete
	default:
		return ValiditySuccess
	}
}

func mergeFailure(r *ClaimResult, a *Answer) {
	if r.Validity != ValidityInvalid {
		if a.Validity == ValidityInvalid || a.Validity == ValidityFailure || r.Validity != ValidityFailure {
			r.Validity = a.Validity
		}
	}
	r.Status = StatusFailed
	if r.Validity == ValidityIncomplete {
		r.Status = StatusSkipped
		r.Reasons = appendUnique(r.Reasons, "execution_incomplete")
	}
	r.Reasons = appendUnique(r.Reasons, a.Error)
}
