package evidence

import (
	"slices"
	"strings"
	"time"
)

func applicability(c *Claim, s *Source, asof string, profile Profile) []string {
	reasons := scopeReasons(&c.Scope, &s.Scope)
	reasons = append(reasons, effectiveDateReasons(c, s, asof)...)
	reasons = append(reasons, snapshotDateReasons(c, s, asof, profile)...)
	return reasons
}

func scopeReasons(target, actual *Scope) []string {
	wanted := []string{target.Entity, target.Definition, target.Conditions, target.Population, target.Version, target.Jurisdiction, target.Context}
	supplied := []string{actual.Entity, actual.Definition, actual.Conditions, actual.Population, actual.Version, actual.Jurisdiction, actual.Context}
	for i, v := range wanted {
		if v != "" && v != supplied[i] {
			return []string{"scope_mismatch"}
		}
	}
	return nil
}

func effectiveDateReasons(c *Claim, s *Source, asof string) []string {
	reasons := []string{}
	if outsideInterval(c.ValidFrom, c.ValidUntil, asof) {
		reasons = append(reasons, "claim_outside_evaluation_date")
	}
	if outsideInterval(s.ValidFrom, s.ValidUntil, asof) {
		reasons = append(reasons, "source_outside_evaluation_date")
	}
	if c.ValidFrom != "" && (s.ValidFrom == "" || s.ValidFrom > c.ValidFrom) {
		reasons = append(reasons, "temporal_coverage_unknown")
	}
	if c.ValidUntil != "" && (s.ValidUntil == "" || s.ValidUntil < c.ValidUntil) {
		reasons = appendUnique(reasons, "temporal_coverage_unknown")
	}
	return reasons
}

func outsideInterval(from, until, asof string) bool {
	return from != "" && asof < from || until != "" && asof > until
}

func snapshotDateReasons(c *Claim, s *Source, asof string, profile Profile) []string {
	reasons := []string{}
	if datePart(s.CapturedAt) > asof {
		reasons = append(reasons, "snapshot_after_evaluation_date")
	}
	if datePart(s.PublishedAt) > asof {
		reasons = append(reasons, "publication_after_evaluation_date")
	}
	requiresDate := c.ValidFrom != "" || c.ValidUntil != "" || slices.Contains(profile.Facets, "freshness")
	if s.CapturedAt == "" && requiresDate {
		reasons = append(reasons, "unknown_snapshot_date")
	}
	return reasons
}

func datePart(value string) string {
	if len(value) == len(time.DateOnly) {
		return value
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return ""
	}
	return t.UTC().Format(time.DateOnly)
}

func (s *session) gateClaims() {
	visited := map[string]bool{}
	for i := range s.p.Claims {
		s.gateClaim(s.p.Claims[i].ID, visited)
	}
}

func (s *session) gateClaim(id string, visited map[string]bool) {
	if visited[id] {
		return
	}
	visited[id] = true
	c := s.claim[id]
	r := s.result[id]
	premises := true
	for _, pid := range c.PremiseIDs {
		s.gateClaim(pid, visited)
		if !s.result[pid].Complete {
			premises = false
			r.Reasons = appendUnique(r.Reasons, "indispensable_premise_unresolved")
		}
	}
	if len(c.AssumptionIDs) > 0 {
		premises = false
		r.Reasons = appendUnique(r.Reasons, "assumption_dependent")
	}
	if !derivationExplicit(c, r) {
		premises = false
		r.Reasons = appendUnique(r.Reasons, "derivation_not_explicit")
	}
	if len(r.SupportingPassageIDs) == 0 {
		r.Reasons = appendUnique(r.Reasons, "no_supporting_anchor")
	}
	if len(r.ContradictingPassageIDs) > 0 {
		r.Reasons = appendUnique(r.Reasons, "contradicting_evidence")
	}
	r.Complete = completeEvidence(r) && premises && !slices.Contains(r.Reasons, "unresolved_evidence_relation")
}

func completeEvidence(r *ClaimResult) bool {
	return r.Status == StatusSuccess && r.Level != nil && *r.Level == supportLevelComplete && len(r.SupportingPassageIDs) > 0 && len(r.ContradictingPassageIDs) == 0
}

func derivationExplicit(c *Claim, r *ClaimResult) bool {
	if r.Directness != "explicit_derivation" && r.Directness != "mixed" {
		return true
	}
	return strings.TrimSpace(c.Derivation) != "" && len(c.PremiseIDs) > 0
}

func (s *session) gateCandidates() {
	for _, c := range s.p.Candidates {
		r := CandidateResult{CandidateID: c.ID, Eligible: true, Requirements: []RequirementResult{}}
		for _, req := range s.p.Requirements {
			rr := s.requirementResult(c, req)
			if rr.Mandatory && !rr.Satisfied {
				r.Eligible = false
				r.Reasons = appendUnique(r.Reasons, "mandatory_requirement_unresolved")
			}
			r.Requirements = append(r.Requirements, rr)
		}
		s.r.Candidates = append(s.r.Candidates, r)
	}
}

func (s *session) requirementResult(c Candidate, req Requirement) RequirementResult {
	ids := c.RequirementClaims[req.ID]
	r := RequirementResult{RequirementID: req.ID, Mandatory: req.Mandatory, Satisfied: len(ids) > 0, ClaimIDs: ids}
	if len(ids) == 0 {
		r.Reasons = append(r.Reasons, "missing_claims")
	}
	for _, id := range ids {
		cr := s.result[id]
		if !cr.Complete {
			r.Satisfied = false
		}
		r.Conflict = r.Conflict || cr.Conflict
		if len(cr.ContradictingPassageIDs) > 0 {
			r.Reasons = appendUnique(r.Reasons, "contradicted_claim")
		}
	}
	return r
}

func (s *session) preChoiceOutcome() string {
	invalid, failure, incomplete := s.executionFailures()
	if invalid {
		return OutcomeInvalidProvider
	}
	if failure {
		return OutcomeProviderFailure
	}
	if incomplete {
		return OutcomeIncomplete
	}
	eligible, conflict := 0, false
	for _, c := range s.r.Candidates {
		if c.Eligible {
			eligible++
		}
		for _, r := range c.Requirements {
			conflict = conflict || mandatoryContradiction(r)
		}
	}
	if eligible == 0 {
		if conflict {
			return OutcomeConflictingEvidence
		}
		return OutcomeMissingEvidence
	}
	if len(s.p.Preferences) == 0 && eligible > 1 {
		return OutcomeMissingPreferences
	}
	return ""
}

func (s *session) executionFailures() (invalid, failure, incomplete bool) {
	for i := range s.r.Claims {
		c := &s.r.Claims[i]
		invalid = invalid || c.Validity == ValidityInvalid
		failure = failure || c.Validity == ValidityFailure
		incomplete = incomplete || slices.Contains(c.Reasons, "execution_incomplete")
	}
	return invalid, failure, incomplete
}

func mandatoryContradiction(r RequirementResult) bool {
	return r.Mandatory && (r.Conflict || slices.Contains(r.Reasons, "contradicted_claim"))
}

func (s *session) choiceCall() Call {
	candidates := []Candidate{}
	options := map[string]string{OutcomeAbstain: "Choose no candidate because the supplied preferences or requirements do not justify a choice", OutcomeInvestigate: "Gather more evidence before choosing"}
	claims := map[string]bool{}
	for _, c := range s.r.Candidates {
		if !c.Eligible {
			continue
		}
		for _, original := range s.p.Candidates {
			if original.ID == c.CandidateID {
				candidates = append(candidates, original)
				options[original.ID] = original.Name
				for _, refs := range original.RequirementClaims {
					for _, id := range refs {
						claims[id] = true
					}
				}
			}
		}
	}
	eligibleClaims := []ClaimResult{}
	for i := range s.r.Claims {
		c := &s.r.Claims[i]
		if claims[c.ClaimID] {
			eligibleClaims = append(eligibleClaims, *c)
		}
	}
	return call("choice", StageChoice, struct {
		Candidates  []Candidate   `json:"eligible_candidates"`
		Preferences []Preference  `json:"preferences"`
		Claims      []ClaimResult `json:"claim_results"`
		Assumptions []Assumption  `json:"assumptions"`
	}{candidates, s.p.Preferences, eligibleClaims, s.p.Assumptions}, options)
}

func (s *session) coverage() {
	s.r.Coverage = Coverage{Total: len(s.r.Claims)}
	for i := range s.r.Claims {
		countClaimCoverage(&s.r.Coverage, &s.r.Claims[i])
	}
	for _, candidate := range s.r.Candidates {
		for _, r := range candidate.Requirements {
			s.countRequirementCoverage(&s.r.RequirementCoverage, r)
		}
	}
}

func countClaimCoverage(c *Coverage, r *ClaimResult) {
	switch r.Status {
	case StatusSkipped:
		c.Skipped++
	case StatusFailed:
		c.Failed++
	default:
		c.Evaluated++
	}
	if r.Complete {
		c.Supported++
	} else {
		c.Missing++
	}
	if len(r.ContradictingPassageIDs) > 0 {
		c.Contradicted++
	}
}

func (s *session) countRequirementCoverage(c *Coverage, r RequirementResult) {
	c.Total++
	status := s.requirementStatus(r.ClaimIDs)
	switch status {
	case StatusSuccess:
		c.Evaluated++
	case StatusFailed:
		c.Failed++
	default:
		c.Skipped++
	}
	if r.Satisfied {
		c.Supported++
	} else {
		c.Missing++
	}
	if r.Conflict || slices.Contains(r.Reasons, "contradicted_claim") {
		c.Contradicted++
	}
}

type sourceGroups struct{ parent map[string]string }

func (g *sourceGroups) root(id string) string {
	if g.parent[id] != id {
		g.parent[id] = g.root(g.parent[id])
	}
	return g.parent[id]
}

func (g *sourceGroups) join(a, b string) {
	a = g.root(a)
	b = g.root(b)
	if a < b {
		g.parent[b] = a
	} else {
		g.parent[a] = b
	}
}

func (g *sourceGroups) link(src *Source, byOrigin, byHash map[string]string) {
	if src.Origin != "" && src.Origin != Unknown {
		if id, ok := byOrigin[src.Origin]; ok {
			g.join(id, src.ID)
		} else {
			byOrigin[src.Origin] = src.ID
		}
	}
	if id, ok := byHash[src.SHA256]; ok {
		g.join(id, src.ID)
	} else {
		byHash[src.SHA256] = src.ID
	}
	for _, id := range src.DerivedFrom {
		g.join(src.ID, id)
	}
}

func origins(sources []Source) map[string]string {
	g := sourceGroups{parent: map[string]string{}}
	for i := range sources {
		g.parent[sources[i].ID] = sources[i].ID
	}
	byOrigin, byHash := map[string]string{}, map[string]string{}
	for i := range sources {
		g.link(&sources[i], byOrigin, byHash)
	}
	known := map[string]bool{}
	for i := range sources {
		src := &sources[i]
		if src.Origin != "" && src.Origin != Unknown {
			known[g.root(src.ID)] = true
		}
	}
	out := map[string]string{}
	for i := range sources {
		src := &sources[i]
		root := g.root(src.ID)
		prefix := Unknown + ":"
		if known[root] {
			prefix = "declared:"
		}
		out[src.ID] = prefix + root
	}
	return out
}

func qualityFacets(p *Plan) []QualityFacet {
	out := make([]QualityFacet, 0, len(p.Profile.Facets)*len(p.Sources))
	for i := range p.Sources {
		src := &p.Sources[i]
		for _, f := range p.Profile.Facets {
			out = append(out, qualityFacet(src, f))
		}
	}
	return out
}

func qualityFacet(src *Source, facet string) QualityFacet {
	q := QualityFacet{SourceID: src.ID, Facet: facet, Status: "unverified"}
	switch facet {
	case "snapshot":
		q.Status = "checked"
		q.Detail = "Supplied UTF-8 bytes are checked against SHA-256 and excerpts"
	case "provenance":
		q.Detail = "Caller-supplied origin and lineage; real independence is unverified"
	case "evidence_kind":
		q.Detail = src.EvidenceKind
	case "method":
		q.Detail = src.Method
	case "authority":
		q.Detail = src.Authority
	case "relevance":
		q.Detail = "Exact declared scope checked; semantic relevance remains advisory"
	case "freshness":
		q.Detail = src.CapturedAt
		if src.CapturedAt == "" {
			q.Status = Unknown
		}
	}
	return q
}

func (s *session) requirementStatus(ids []string) string {
	if len(ids) == 0 {
		return StatusSkipped
	}
	status := StatusSuccess
	for _, id := range ids {
		v := s.result[id]
		if v.Status == StatusFailed {
			return StatusFailed
		}
		if v.Status != StatusSuccess {
			status = StatusSkipped
		}
	}
	return status
}
