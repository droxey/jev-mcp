// Package evidence evaluates documentary support in caller-supplied snapshots.
// It never retrieves source URLs or paths and never treats a model score as truth.
package evidence

import (
	"context"
	"encoding/json"
	"time"
)

const (
	Unknown                    = "unknown"
	severityError              = "error"
	OutcomeIncomplete          = "incomplete"
	supportLevelComplete       = 2
	ValiditySuccess            = "success"
	ValidityIncomplete         = "incomplete"
	ValidityInvalid            = "invalid_response"
	ValidityFailure            = "provider_failure"
	SchemaVersion              = "evidence-plan-v1"
	RubricVersion              = "documentary-support-v1"
	StageRelation              = "relation"
	StageSufficiency           = "sufficiency"
	StageChoice                = "choice"
	StageDirectness            = "directness"
	PolicyVersion              = "evidence-policy-v1"
	Supports                   = "supports"
	Contradicts                = "contradicts"
	Insufficient               = "insufficient_information"
	Ambiguous                  = "ambiguous"
	StatusSuccess              = "success"
	StatusFailed               = "failed"
	StatusSkipped              = "skipped"
	OutcomeSelected            = "selected"
	OutcomeAbstain             = "abstain"
	OutcomeInvestigate         = "investigate"
	OutcomeMissingPreferences  = "missing_preferences"
	OutcomeMissingEvidence     = "missing_evidence"
	OutcomeConflictingEvidence = "conflicting_evidence"
	OutcomeInvalidProvider     = "invalid_provider_output"
	OutcomeProviderFailure     = "provider_failure"
)

// Scope fields are exact applicability constraints, not free-text similarity.
// Empty claim fields impose no constraint; missing source fields cannot satisfy one.
type Scope struct {
	Entity       string `json:"entity,omitempty"`
	Definition   string `json:"definition,omitempty"`
	Conditions   string `json:"conditions,omitempty"`
	Population   string `json:"population,omitempty"`
	Version      string `json:"version,omitempty"`
	Jurisdiction string `json:"jurisdiction,omitempty"`
	Context      string `json:"context,omitempty"`
}

type Plan struct {
	Version      string        `json:"version"`
	ID           string        `json:"id"`
	Rubric       string        `json:"rubric"`
	AsOf         string        `json:"as_of"`
	Profile      Profile       `json:"profile"`
	Requirements []Requirement `json:"requirements"`
	Preferences  []Preference  `json:"preferences,omitempty"`
	Candidates   []Candidate   `json:"candidates"`
	Claims       []Claim       `json:"claims"`
	Sources      []Source      `json:"sources"`
	Passages     []Passage     `json:"passages"`
	Assumptions  []Assumption  `json:"assumptions,omitempty"`
}

// Profile selects explicit review facets, not a universal source-quality score.
type Profile struct {
	Version string   `json:"version"`
	Name    string   `json:"name"`
	Facets  []string `json:"facets"`
}

type Requirement struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	Mandatory bool   `json:"mandatory"`
}

type Preference struct {
	ID     string  `json:"id"`
	Text   string  `json:"text"`
	Weight float64 `json:"weight"`
}

type Candidate struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// RequirementClaims maps each evaluated requirement to its indispensable claims.
	RequirementClaims map[string][]string `json:"requirement_claims"`
}

type Claim struct {
	Derivation    string   `json:"derivation,omitempty"`
	ID            string   `json:"id"`
	Text          string   `json:"text"`
	Scope         Scope    `json:"scope"`
	ValidFrom     string   `json:"valid_from,omitempty"`
	ValidUntil    string   `json:"valid_until,omitempty"`
	PassageIDs    []string `json:"passage_ids"`
	PremiseIDs    []string `json:"premise_ids,omitempty"`
	AssumptionIDs []string `json:"assumption_ids,omitempty"`
}

type Source struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	EvidenceKind string `json:"evidence_kind"`
	Method       string `json:"method,omitempty"`
	Authority    string `json:"authority,omitempty"`
	// URI is provenance metadata only; it is never dereferenced.
	URI    string `json:"uri,omitempty"`
	Origin string `json:"origin"`
	// DerivedFrom identifies supplied source IDs copied or derived from this source.
	DerivedFrom []string `json:"derived_from,omitempty"`
	Snapshot    string   `json:"snapshot"`
	SHA256      string   `json:"sha256"`
	CapturedAt  string   `json:"captured_at"`
	PublishedAt string   `json:"published_at,omitempty"`
	Scope       Scope    `json:"scope"`
	ValidFrom   string   `json:"valid_from,omitempty"`
	ValidUntil  string   `json:"valid_until,omitempty"`
}

type Passage struct {
	ID        string `json:"id"`
	SourceID  string `json:"source_id"`
	StartByte int    `json:"start_byte"`
	EndByte   int    `json:"end_byte"`
	Excerpt   string `json:"excerpt"`
}

type Assumption struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	// Accepted makes a user assumption explicit; it never establishes a premise.
	Accepted bool `json:"accepted"`
}

type Issue struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Path     string `json:"path"`
	Message  string `json:"message"`
}

// Provider receives fixed-option structured questions. It must respect ctx and
// return immutable/resolved identity when known; an absent identity stays unknown.
type Provider interface {
	Evaluate(context.Context, []Call) ([]Answer, error)
}

type Call struct {
	ID      string            `json:"id"`
	Stage   string            `json:"stage"`
	State   json.RawMessage   `json:"state"`
	Options map[string]string `json:"options"`
}

type Answer struct {
	ID       string `json:"id"`
	Selected string `json:"selected,omitempty"`
	// SemanticLevel is set only after verified mapping of the provider's legend.
	SemanticLevel  *int               `json:"semantic_level,omitempty"`
	Score          *float64           `json:"score,omitempty"`
	Probability    *float64           `json:"probability,omitempty"`
	Confidence     *float64           `json:"confidence,omitempty"`
	Probabilities  map[string]float64 `json:"probabilities,omitempty"`
	Raw            json.RawMessage    `json:"raw,omitempty"`
	RawEncoding    string             `json:"raw_encoding,omitempty"`
	RawSHA256      string             `json:"raw_sha256,omitempty"`
	Provider       string             `json:"provider,omitempty"`
	RequestedModel string             `json:"requested_model,omitempty"`
	ResolvedModel  string             `json:"resolved_model,omitempty"`
	Status         string             `json:"status"`
	Validity       string             `json:"validity"`
	Error          string             `json:"error,omitempty"`
}

type Limits struct {
	Timeout       time.Duration
	MaxCalls      int
	BatchSize     int
	Concurrency   int
	MaxRetries    int
	MaxInputBytes int
}

type PassageResult struct {
	PassageID  string   `json:"passage_id"`
	Relation   string   `json:"relation,omitempty"`
	Applicable bool     `json:"applicable"`
	Origin     string   `json:"origin"`
	Reasons    []string `json:"reasons,omitempty"`
	Answer     *Answer  `json:"answer,omitempty"`
}

type ClaimResult struct {
	ClaimID                 string          `json:"claim_id"`
	Text                    string          `json:"text"`
	Scope                   Scope           `json:"scope"`
	Status                  string          `json:"status"`
	Validity                string          `json:"validity"`
	Level                   *int            `json:"level,omitempty"`
	Complete                bool            `json:"complete"`
	Directness              string          `json:"directness"`
	DirectnessAnswer        *Answer         `json:"directness_answer,omitempty"`
	Conflict                bool            `json:"conflict"`
	Passages                []PassageResult `json:"passages"`
	SupportingPassageIDs    []string        `json:"supporting_passage_ids"`
	ContradictingPassageIDs []string        `json:"contradicting_passage_ids"`
	SupportingOrigins       []string        `json:"supporting_origins"`
	ContradictingOrigins    []string        `json:"contradicting_origins"`
	Reasons                 []string        `json:"reasons,omitempty"`
	Answer                  *Answer         `json:"answer,omitempty"`
}

type RequirementResult struct {
	RequirementID string   `json:"requirement_id"`
	Mandatory     bool     `json:"mandatory"`
	Satisfied     bool     `json:"satisfied"`
	Conflict      bool     `json:"conflict"`
	ClaimIDs      []string `json:"claim_ids"`
	Reasons       []string `json:"reasons,omitempty"`
}

type CandidateResult struct {
	CandidateID  string              `json:"candidate_id"`
	Eligible     bool                `json:"eligible"`
	Requirements []RequirementResult `json:"requirements"`
	Reasons      []string            `json:"reasons,omitempty"`
}

type Coverage struct {
	Failed       int `json:"failed"`
	Total        int `json:"total"`
	Evaluated    int `json:"evaluated"`
	Supported    int `json:"supported"`
	Contradicted int `json:"contradicted"`
	Missing      int `json:"missing"`
	Skipped      int `json:"skipped"`
}

type QualityFacet struct {
	SourceID string `json:"source_id"`
	Facet    string `json:"facet"`
	Status   string `json:"status"`
	Detail   string `json:"detail,omitempty"`
}

type Report struct {
	Validity            string            `json:"validity"`
	Citations           []Citation        `json:"citations"`
	Sources             []SourceRecord    `json:"sources"`
	Models              []ModelIdentity   `json:"models"`
	Version             string            `json:"version"`
	Rubric              string            `json:"rubric"`
	PlanID              string            `json:"plan_id"`
	Policy              string            `json:"policy"`
	Calibration         string            `json:"calibration"`
	Profile             Profile           `json:"profile"`
	Coverage            Coverage          `json:"coverage"`
	RequirementCoverage Coverage          `json:"requirement_coverage"`
	Quality             []QualityFacet    `json:"quality"`
	PlanSHA256          string            `json:"plan_sha256"`
	Outcome             string            `json:"outcome"`
	SelectedID          string            `json:"selected_id,omitempty"`
	Issues              []Issue           `json:"issues,omitempty"`
	Claims              []ClaimResult     `json:"claims"`
	Candidates          []CandidateResult `json:"candidates"`
	Choice              *Answer           `json:"choice,omitempty"`
	Assumptions         []Assumption      `json:"assumptions,omitempty"`
	Calls               int               `json:"calls"`
}
