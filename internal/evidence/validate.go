package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"
)

// Plan limits bound validation and provider work before any model call.
const (
	MaxPlanBytes  = 1 << 20
	MaxClaims     = 128
	MaxSources    = 64
	MaxPassages   = 256
	MaxCandidates = 64
)

const (
	maxIdentifierBytes   = 128
	maxRecordTextBytes   = 16 << 10
	maxSnapshotBytes     = 256 << 10
	maxRequirements      = 128
	maxPreferences       = 64
	maxAssumptions       = 128
	maxProfileFacets     = 7
	jsonStringDelimiters = 2
	unknownSourceCode    = "unknown_source"
	unknownSourceMessage = "source reference does not exist"
)

// DecodePlan rejects oversized input, unknown fields, and trailing JSON values.
func DecodePlan(r io.Reader) (Plan, error) {
	b, err := io.ReadAll(io.LimitReader(r, MaxPlanBytes+1))
	if err != nil {
		return Plan{}, err
	}
	if len(b) > MaxPlanBytes {
		return Plan{}, errors.New("evidence: plan exceeds byte limit")
	}
	var p Plan
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	if err := d.Decode(&p); err != nil {
		return Plan{}, fmt.Errorf("evidence: invalid plan JSON: %w", err)
	}
	var extra any
	if err := d.Decode(&extra); !errors.Is(err, io.EOF) {
		return Plan{}, errors.New("evidence: expected exactly one JSON value")
	}
	return p, nil
}

type planLinter struct {
	issues       []Issue
	ids          map[string]bool
	requirements map[string]bool
	claims       map[string]Claim
	sources      map[string]Source
	passages     map[string]Passage
	assumptions  map[string]bool
}

// Lint checks syntax and supplied provenance; warnings cannot certify semantics.
// No URI or filename in a plan is opened or fetched.
func Lint(p *Plan) []Issue {
	l := &planLinter{issues: []Issue{}, ids: map[string]bool{}, requirements: map[string]bool{}, claims: map[string]Claim{}, sources: map[string]Source{}, passages: map[string]Passage{}, assumptions: map[string]bool{}}
	l.metadata(p)
	if !l.withinBounds(p) {
		return l.issues
	}
	l.requirementRecords(p.Requirements)
	l.preferenceRecords(p.Preferences)
	l.assumptionRecords(p.Assumptions)
	l.claimRecords(p.Claims)
	l.sourceRecords(p.Sources)
	l.passageRecords(p.Passages)
	l.claimReferences(p.Claims)
	l.sourceReferences(p.Sources)
	l.candidateRecords(p.Candidates)
	l.cycles()
	return l.issues
}

func (l *planLinter) errorAt(code, path, message string) {
	l.issues = append(l.issues, Issue{Severity: severityError, Code: code, Path: path, Message: message})
}

func (l *planLinter) warn(code, path, message string) {
	l.issues = append(l.issues, Issue{Severity: "warning", Code: code, Path: path, Message: message})
}

func (l *planLinter) metadata(p *Plan) {
	if p.Version != SchemaVersion {
		l.errorAt("schema_version", "version", "unsupported schema version")
	}
	if p.Rubric != RubricVersion {
		l.errorAt("rubric_version", "rubric", "unsupported documentary rubric")
	}
	l.profile(p.Profile)
	if p.ID == "" {
		l.errorAt("required", "id", "plan ID is required")
	} else {
		l.id(p.ID, "id")
	}
	l.date(p.AsOf, "as_of")
	if p.AsOf == "" {
		l.errorAt("required", "as_of", "evaluation date is required")
	}
}

func (l *planLinter) profile(profile Profile) {
	if profile.Version != "evidence-profile-v1" {
		l.errorAt("profile_version", "profile.version", "unsupported evidence profile")
	}
	if strings.TrimSpace(profile.Name) == "" {
		l.errorAt("required", "profile.name", "profile name is required")
	}
	if len(profile.Facets) > maxProfileFacets {
		l.errorAt("profile_facet_limit", "profile.facets", "at most seven unique quality facets are allowed")
		return
	}
	seen := map[string]bool{}
	facets := map[string]bool{"provenance": true, "snapshot": true, "evidence_kind": true, "method": true, "authority": true, "relevance": true, "freshness": true}
	for i, f := range profile.Facets {
		if seen[f] {
			l.errorAt("duplicate_facet", fmt.Sprintf("profile.facets[%d]", i), "quality facets must be unique")
		}
		seen[f] = true
		if !facets[f] {
			l.errorAt("unknown_facet", fmt.Sprintf("profile.facets[%d]", i), "unsupported quality facet")
		}
	}
}

func (l *planLinter) withinBounds(p *Plan) bool {
	if len(p.Candidates) == 0 {
		l.errorAt("empty_candidates", "candidates", "at least one candidate is required")
	}
	if len(p.Requirements) == 0 {
		l.errorAt("empty_requirements", "requirements", "at least one requirement is required")
	}
	if len(p.Candidates) > MaxCandidates || len(p.Claims) > MaxClaims || len(p.Sources) > MaxSources || len(p.Passages) > MaxPassages || len(p.Requirements) > maxRequirements || len(p.Preferences) > maxPreferences || len(p.Assumptions) > maxAssumptions {
		l.errorAt("plan_limit", "", "plan collection exceeds limit")
		return false
	}
	if len(p.Profile.Facets) > maxProfileFacets || !minimumSizeWithinLimit(p) {
		l.errorAt("plan_limit", "", "plan exceeds bounded input size")
		return false
	}
	b, err := json.Marshal(p)
	if err != nil || len(b) > MaxPlanBytes {
		l.errorAt("plan_limit", "", "plan exceeds serialized byte limit")
		return false
	}
	return true
}

func (l *planLinter) id(value, path string) {
	if !validIdentifier(value) {
		l.errorAt("invalid_id", path, "ID must contain 1 to 128 ASCII letters, digits, dots, underscores, or hyphens")
	}
	if l.ids[value] {
		l.errorAt("duplicate_id", path, "IDs must be globally unique")
	}
	l.ids[value] = true
}

func validIdentifier(value string) bool {
	if value == "" || len(value) > maxIdentifierBytes {
		return false
	}
	for _, c := range value {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '_' || c == '-' {
			continue
		}
		return false
	}
	return true
}

func (l *planLinter) text(value, path string) {
	if strings.TrimSpace(value) == "" {
		l.errorAt("required", path, "nonblank text is required")
	}
	if len(value) > maxRecordTextBytes {
		l.errorAt("text_limit", path, "text exceeds 16384 bytes")
	}
}

func (l *planLinter) date(value, path string) {
	if value == "" {
		return
	}
	if _, err := time.Parse(time.DateOnly, value); err != nil {
		l.errorAt("invalid_date", path, "expected YYYY-MM-DD")
	}
}

func (l *planLinter) interval(from, until, path string) {
	l.date(from, path+".valid_from")
	l.date(until, path+".valid_until")
	if from != "" && until != "" && from > until {
		l.errorAt("invalid_interval", path, "valid_from exceeds valid_until")
	}
}

func (l *planLinter) requirementRecords(records []Requirement) {
	for i, r := range records {
		path := fmt.Sprintf("requirements[%d]", i)
		l.id(r.ID, path+".id")
		l.text(r.Text, path+".text")
		l.requirements[r.ID] = true
	}
}

func (l *planLinter) preferenceRecords(records []Preference) {
	for i, r := range records {
		path := fmt.Sprintf("preferences[%d]", i)
		l.id(r.ID, path+".id")
		l.text(r.Text, path+".text")
		if math.IsNaN(r.Weight) || math.IsInf(r.Weight, 0) || r.Weight <= 0 {
			l.errorAt("preference_weight", path+".weight", "weight must be positive and finite")
		}
	}
}

func (l *planLinter) assumptionRecords(records []Assumption) {
	for i, a := range records {
		path := fmt.Sprintf("assumptions[%d]", i)
		l.id(a.ID, path+".id")
		l.text(a.Text, path+".text")
		l.assumptions[a.ID] = true
	}
}

func (l *planLinter) claimRecords(records []Claim) {
	for i := range records {
		c := &records[i]
		path := fmt.Sprintf("claims[%d]", i)
		l.id(c.ID, path+".id")
		l.text(c.Text, path+".text")
		l.interval(c.ValidFrom, c.ValidUntil, path)
		l.claims[c.ID] = *c
		l.claimHeuristics(c.Text, path)
	}
}

func (l *planLinter) claimHeuristics(text, path string) {
	lower := strings.ToLower(text)
	if strings.Contains(lower, " and ") || strings.Contains(lower, "; ") || strings.Contains(lower, " or ") {
		l.warn("compound_claim", path+".text", "may contain multiple claims; manual atomization review required")
	}
	if strings.Contains(lower, "ignore previous") || strings.Contains(lower, "system prompt") || strings.Contains(lower, "disregard instructions") {
		l.warn("untrusted_instruction", path+".text", "instruction-like source content remains untrusted data")
	}
}

func (l *planLinter) sourceRecords(records []Source) {
	for i := range records {
		s := &records[i]
		path := fmt.Sprintf("sources[%d]", i)
		l.id(s.ID, path+".id")
		l.text(s.Title, path+".title")
		l.text(s.Origin, path+".origin")
		l.sources[s.ID] = *s
		l.interval(s.ValidFrom, s.ValidUntil, path)
		l.snapshot(s, path)
		l.sourceDates(s, path)
		switch s.EvidenceKind {
		case "assertion", "observation", "artifact", "calculation", Unknown:
		default:
			l.errorAt("evidence_kind", path+".evidence_kind", "expected assertion, observation, artifact, calculation, or unknown")
		}
	}
}

func (l *planLinter) snapshot(s *Source, path string) {
	if s.Snapshot == "" || len(s.Snapshot) > maxSnapshotBytes || !utf8.ValidString(s.Snapshot) {
		l.errorAt("snapshot_limit", path+".snapshot", "snapshot must contain 1 to 262144 valid UTF-8 bytes")
	}
	digest := sha256.Sum256([]byte(s.Snapshot))
	if s.SHA256 != "" && s.SHA256 != hex.EncodeToString(digest[:]) {
		l.errorAt("snapshot_hash", path+".sha256", "snapshot SHA-256 does not match supplied UTF-8 bytes")
	}
}

func (l *planLinter) sourceDates(s *Source, path string) {
	for name, value := range map[string]string{"captured_at": s.CapturedAt, "published_at": s.PublishedAt} {
		if value == "" {
			continue
		}
		if _, err := time.Parse(time.RFC3339, value); err != nil {
			if _, dateErr := time.Parse(time.DateOnly, value); dateErr != nil {
				l.errorAt("invalid_date", path+"."+name, "expected YYYY-MM-DD or RFC3339 timestamp")
			}
		}
	}
	if s.CapturedAt == "" {
		l.warn("unknown_snapshot_date", path+".captured_at", "capture date is unknown")
	}
}

func (l *planLinter) passageRecords(records []Passage) {
	for i, p := range records {
		path := fmt.Sprintf("passages[%d]", i)
		l.id(p.ID, path+".id")
		l.passages[p.ID] = p
		s, ok := l.sources[p.SourceID]
		if !ok {
			l.errorAt(unknownSourceCode, path+".source_id", unknownSourceMessage)
			continue
		}
		l.passage(p, s.Snapshot, path)
	}
}

func (l *planLinter) passage(p Passage, snapshot, path string) {
	if p.StartByte < 0 || p.EndByte <= p.StartByte || p.EndByte > len(snapshot) || !utf8.ValidString(snapshot[:max(0, min(p.StartByte, len(snapshot)))]) || !utf8.ValidString(snapshot[:max(0, min(p.EndByte, len(snapshot)))]) {
		l.errorAt("byte_range", path, "range must be nonempty half-open UTF-8 byte boundaries in snapshot")
		return
	}
	if snapshot[p.StartByte:p.EndByte] != p.Excerpt {
		l.errorAt("excerpt_mismatch", path+".excerpt", "excerpt differs from exact snapshot bytes")
	}
}

func (l *planLinter) duplicateReferences(refs []string, path string) {
	seen := map[string]bool{}
	for _, ref := range refs {
		if seen[ref] {
			l.errorAt("duplicate_reference", path, "duplicate reference")
		}
		seen[ref] = true
	}
}

func (l *planLinter) claimReferences(records []Claim) {
	for i := range records {
		c := &records[i]
		path := fmt.Sprintf("claims[%d]", i)
		for _, ref := range c.PassageIDs {
			if _, ok := l.passages[ref]; !ok {
				l.errorAt("unknown_passage", path+".passage_ids", "passage reference does not exist")
			}
		}
		for _, ref := range c.PremiseIDs {
			if _, ok := l.claims[ref]; !ok {
				l.errorAt("unknown_premise", path+".premise_ids", "premise reference does not exist")
			}
		}
		for _, ref := range c.AssumptionIDs {
			if !l.assumptions[ref] {
				l.errorAt("unknown_assumption", path+".assumption_ids", "assumption reference does not exist")
			}
		}
		l.duplicateReferences(c.PassageIDs, path+".passage_ids")
		l.duplicateReferences(c.PremiseIDs, path+".premise_ids")
	}
}

func (l *planLinter) sourceReferences(records []Source) {
	for i := range records {
		s := &records[i]
		for _, ref := range s.DerivedFrom {
			if _, ok := l.sources[ref]; !ok {
				l.errorAt(unknownSourceCode, fmt.Sprintf("sources[%d].derived_from", i), unknownSourceMessage)
			}
		}
	}
}

func (l *planLinter) candidateRecords(records []Candidate) {
	for i, c := range records {
		path := fmt.Sprintf("candidates[%d]", i)
		l.id(c.ID, path+".id")
		l.text(c.Name, path+".name")
		if c.ID == OutcomeAbstain || c.ID == OutcomeInvestigate {
			l.errorAt("reserved_id", path+".id", "candidate ID is reserved for policy outcomes")
		}
		l.candidateReferences(c, path)
	}
}

func (l *planLinter) candidateReferences(c Candidate, path string) {
	for requirement, refs := range c.RequirementClaims {
		if !l.requirements[requirement] {
			l.errorAt("unknown_requirement", path+".requirement_claims", "requirement reference does not exist")
		}
		l.duplicateReferences(refs, path+".requirement_claims")
		for _, ref := range refs {
			if _, ok := l.claims[ref]; !ok {
				l.errorAt("unknown_claim", path+".requirement_claims", "claim reference does not exist")
			}
		}
	}
}

func (l *planLinter) cycles() {
	claims := map[string][]string{}
	sources := map[string][]string{}
	for id := range l.claims {
		claims[id] = l.claims[id].PremiseIDs
	}
	for id := range l.sources {
		sources[id] = l.sources[id].DerivedFrom
	}
	if cycle(claims) {
		l.errorAt("premise_cycle", "claims", "indispensable premise graph must be acyclic")
	}
	if cycle(sources) {
		l.errorAt("source_cycle", "sources", "source derivation graph must be acyclic")
	}
}

func cycle(g map[string][]string) bool {
	const visiting, visited = 1, 2
	state := map[string]int{}
	var visit func(string) bool
	visit = func(id string) bool {
		if state[id] == visiting {
			return true
		}
		if state[id] == visited {
			return false
		}
		state[id] = visiting
		for _, next := range g[id] {
			if visit(next) {
				return true
			}
		}
		state[id] = visited
		return false
	}
	for id := range g {
		if visit(id) {
			return true
		}
	}
	return false
}

// minimumSizeWithinLimit rejects huge in-memory inputs before JSON serialization.
// DecodePlan already bounds wire bytes; this also protects direct Go callers.
func minimumSizeWithinLimit(p *Plan) bool {
	remaining := MaxPlanBytes
	return boundedValue(reflect.ValueOf(p).Elem(), &remaining)
}

func boundedValue(v reflect.Value, remaining *int) bool {
	*remaining--
	if *remaining < 0 {
		return false
	}
	switch v.Kind() {
	case reflect.String:
		*remaining -= v.Len() + jsonStringDelimiters
	case reflect.Struct:
		return boundedStruct(v, remaining)
	case reflect.Slice:
		return boundedSlice(v, remaining)
	case reflect.Map:
		return boundedMap(v, remaining)
	default:
	}
	return *remaining >= 0
}

func boundedStruct(v reflect.Value, remaining *int) bool {
	for _, field := range v.Fields() {
		if !boundedValue(field, remaining) {
			return false
		}
	}
	return true
}

func boundedSlice(v reflect.Value, remaining *int) bool {
	if v.Len() > *remaining {
		return false
	}
	for i := 0; i < v.Len(); i++ {
		if !boundedValue(v.Index(i), remaining) {
			return false
		}
	}
	return true
}

func boundedMap(v reflect.Value, remaining *int) bool {
	if v.Len() > *remaining {
		return false
	}
	iter := v.MapRange()
	for iter.Next() {
		if !boundedValue(iter.Key(), remaining) || !boundedValue(iter.Value(), remaining) {
			return false
		}
	}
	return true
}
