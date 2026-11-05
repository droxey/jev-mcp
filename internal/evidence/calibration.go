package evidence

import (
	"errors"
	"fmt"
	"slices"
)

const (
	worstOtherRank = 3
	maxThresholds  = 64
)

// CalibrationSet contains manually labeled local reports; it performs no calls.
type CalibrationSet struct {
	Version string            `json:"version"`
	Rubric  string            `json:"rubric"`
	Policy  string            `json:"policy"`
	Cases   []CalibrationCase `json:"cases"`
}
type CalibrationCase struct {
	ID              string       `json:"id"`
	Split           string       `json:"split"`
	LabelSource     string       `json:"label_source"`
	Expected        []ClaimLabel `json:"expected"`
	ExpectedAbstain bool         `json:"expected_abstain"`
	Report          Report       `json:"report"`
}
type ClaimLabel struct {
	ExpectedAcceptedComplete *bool      `json:"expected_accepted_complete,omitempty"`
	CompleteEvidenceGroups   [][]string `json:"complete_evidence_groups,omitempty"`
	ClaimID                  string     `json:"claim_id"`
	Level                    int        `json:"level"`
	Contradiction            bool       `json:"contradiction"`
	Conflict                 bool       `json:"conflict"`
}
type CalibrationReport struct {
	AcceptanceErrors         int               `json:"acceptance_errors"`
	CompleteGroupEvaluated   int               `json:"complete_group_evaluated"`
	CompleteGroupCorrect     int               `json:"complete_group_correct"`
	IncompleteEvidenceGroups int               `json:"incomplete_evidence_groups"`
	Models                   []ModelIdentity   `json:"models"`
	IdentityChanged          bool              `json:"identity_changed"`
	Calibration              string            `json:"calibration"`
	Cases                    int               `json:"cases"`
	ExcludedTraining         int               `json:"excluded_training"`
	FalseSupport             int               `json:"false_support"`
	MissedSupport            int               `json:"missed_support"`
	MissedContradictions     int               `json:"missed_contradictions"`
	FalseConflicts           int               `json:"false_conflicts"`
	Abstentions              int               `json:"abstentions"`
	AbstentionErrors         int               `json:"abstention_errors"`
	Coverage                 float64           `json:"coverage"`
	WorstMisses              []CalibrationMiss `json:"worst_misses"`
	Sweeps                   []ThresholdResult `json:"threshold_sweeps"`
}
type CalibrationMiss struct {
	CaseID   string   `json:"case_id"`
	ClaimID  string   `json:"claim_id,omitempty"`
	Kind     string   `json:"kind"`
	Strength *float64 `json:"strength,omitempty"`
}
type ThresholdResult struct {
	Threshold       float64 `json:"threshold"`
	FalseSupport    int     `json:"false_support"`
	MissedSupport   int     `json:"missed_support"`
	Classified      int     `json:"classified"`
	UnknownStrength int     `json:"unknown_strength"`
}

// Calibrate reports held-out error counts. Thresholds are diagnostic sweeps,
// never configuration updates or learned production cutoffs.
func Calibrate(s CalibrationSet, thresholds []float64) (CalibrationReport, error) {
	if err := validateCalibration(s, thresholds); err != nil {
		return CalibrationReport{}, err
	}
	r := CalibrationReport{Calibration: "advisory_not_a_calibrated_cutoff", Models: []ModelIdentity{}, WorstMisses: []CalibrationMiss{}, Sweeps: make([]ThresholdResult, len(thresholds))}
	for i, t := range thresholds {
		r.Sweeps[i].Threshold = t
	}

	total, covered := 0, 0
	for i := range s.Cases {
		c := &s.Cases[i]
		if c.Split == "train" {
			r.ExcludedTraining++
			continue
		}
		n, k := measureCase(&r, c)
		total += n
		covered += k
	}
	r.IdentityChanged = len(r.Models) > 1
	if total > 0 {
		r.Coverage = float64(covered) / float64(total)
	}
	// Most dangerous false-support cases appear first, then missed contradictions.
	slices.SortStableFunc(r.WorstMisses, func(a, b CalibrationMiss) int { return missRank(a.Kind) - missRank(b.Kind) })
	return r, nil
}

func measureCase(r *CalibrationReport, c *CalibrationCase) (total, covered int) {
	r.Cases++
	for _, id := range c.Report.Models {
		if !slices.Contains(r.Models, id) {
			r.Models = append(r.Models, id)
		}
	}
	abstain := c.Report.Outcome != OutcomeSelected
	if abstain {
		r.Abstentions++
	}
	if abstain != c.ExpectedAbstain {
		r.AbstentionErrors++
		r.WorstMisses = append(r.WorstMisses, CalibrationMiss{CaseID: c.ID, Kind: "abstention_error"})
	}
	actual := map[string]*ClaimResult{}
	for i := range c.Report.Claims {
		v := &c.Report.Claims[i]
		actual[v.ClaimID] = v
	}
	for _, label := range c.Expected {
		v := actual[label.ClaimID]
		total++
		if v.Status == StatusSuccess {
			covered++
		}
		measureClaim(r, c.ID, label, v)
		for i := range r.Sweeps {
			measureThreshold(&r.Sweeps[i], label, v)
		}
	}
	return total, covered
}

func validateCalibration(s CalibrationSet, thresholds []float64) error {
	if s.Version != "evidence-calibration-v1" || s.Rubric != RubricVersion || s.Policy != PolicyVersion {
		return errors.New("evidence: incompatible calibration versions")
	}
	if len(s.Cases) == 0 || len(s.Cases) > 1024 {
		return errors.New("evidence: calibration requires 1 to 1024 labeled cases")
	}
	if err := validateThresholds(thresholds); err != nil {
		return err
	}

	heldout := 0
	ids := map[string]bool{}
	for i := range s.Cases {
		c := &s.Cases[i]
		if !compatibleRecordedReport(&c.Report) {
			return errors.New("evidence: recorded report schema, rubric, or policy is stale")
		}
		if !validCalibrationCase(c, ids) {
			return errors.New("evidence: each case requires unique ID, original label source, and claim labels")
		}
		ids[c.ID] = true
		if c.Split != "heldout" && c.Split != "train" {
			return errors.New("evidence: split must be heldout or train")
		}
		if c.Split == "heldout" {
			heldout++
		}
		if err := validateLabels(c); err != nil {
			return err
		}
	}
	if heldout == 0 {
		return errors.New("evidence: at least one heldout case is required")
	}
	return nil
}

func validateLabels(c *CalibrationCase) error {
	claims := map[string]bool{}
	for i := range c.Report.Claims {
		v := &c.Report.Claims[i]
		if claims[v.ClaimID] {
			return errors.New("evidence: duplicate observed claim ID")
		}
		claims[v.ClaimID] = true
	}
	seen := map[string]bool{}
	for _, v := range c.Expected {
		if v.Level < 0 || v.Level > 2 || !claims[v.ClaimID] || seen[v.ClaimID] {
			return fmt.Errorf("evidence: invalid or missing claim label in case %q", c.ID)
		}
		if err := validateGoldGroups(v); err != nil {
			return err
		}
		seen[v.ClaimID] = true
	}
	return nil
}

func strength(v *ClaimResult) *float64 {
	if v.Answer == nil {
		return nil
	}
	p, ok := v.Answer.Probabilities["2"]
	if !ok || !finite(p) || p < 0 || p > 1 {
		return nil
	}
	return &p
}

func measureClaim(r *CalibrationReport, id string, label ClaimLabel, v *ClaimResult) {
	measureGoldGroups(r, id, label, v)
	expected := label.Level == supportLevelComplete && !label.Contradiction && !label.Conflict
	if label.ExpectedAcceptedComplete != nil {
		expected = *label.ExpectedAcceptedComplete
	}
	if v.Complete != expected {
		r.AcceptanceErrors++
		r.WorstMisses = append(r.WorstMisses, CalibrationMiss{CaseID: id, ClaimID: label.ClaimID, Kind: "acceptance_error", Strength: strength(v)})
	}
	add := func(kind string) {
		r.WorstMisses = append(r.WorstMisses, CalibrationMiss{CaseID: id, ClaimID: label.ClaimID, Kind: kind, Strength: strength(v)})
	}
	if v.Level != nil && *v.Level == supportLevelComplete && label.Level < supportLevelComplete {
		r.FalseSupport++
		add("false_support")
	}
	if v.Level != nil && *v.Level != supportLevelComplete && label.Level == supportLevelComplete {
		r.MissedSupport++
		add("missed_support")
	}
	if label.Contradiction && len(v.ContradictingPassageIDs) == 0 {
		r.MissedContradictions++
		add("missed_contradiction")
	}
	if !label.Conflict && v.Conflict {
		r.FalseConflicts++
		add("false_conflict")
	}
}

func measureThreshold(r *ThresholdResult, label ClaimLabel, v *ClaimResult) {
	p := strength(v)
	if p == nil {
		r.UnknownStrength++
		return
	}
	r.Classified++
	supported := *p >= r.Threshold
	if supported && label.Level < 2 {
		r.FalseSupport++
	}
	if !supported && label.Level == 2 {
		r.MissedSupport++
	}
}

func missRank(kind string) int {
	switch kind {
	case "false_support":
		return 0
	case "missed_contradiction":
		return 1
	case "false_conflict":
		return supportLevelComplete
	default:
		return worstOtherRank
	}
}

func validateThresholds(thresholds []float64) error {
	if len(thresholds) > maxThresholds {
		return errors.New("evidence: at most 64 diagnostic thresholds are allowed")
	}
	for _, t := range thresholds {
		if !finite(t) || t < 0 || t > 1 {
			return errors.New("evidence: threshold must be finite in [0,1]")
		}
	}
	return nil
}

func validCalibrationCase(c *CalibrationCase, ids map[string]bool) bool {
	return c.ID != "" && !ids[c.ID] && c.LabelSource != "" && len(c.Expected) > 0 && len(c.Expected) <= MaxClaims && len(c.Report.Claims) <= MaxClaims
}

func validateGoldGroups(label ClaimLabel) error {
	if len(label.CompleteEvidenceGroups) > MaxPassages {
		return errors.New("evidence: too many gold evidence groups")
	}
	total := 0
	for _, group := range label.CompleteEvidenceGroups {
		total += len(group)
		if len(group) == 0 || total > MaxPassages {
			return errors.New("evidence: gold groups must be nonempty and bounded")
		}
		seen := map[string]bool{}
		for _, id := range group {
			if !validIdentifier(id) || seen[id] {
				return errors.New("evidence: gold group IDs must be valid and unique")
			}
			seen[id] = true
		}
	}
	return nil
}

func measureGoldGroups(r *CalibrationReport, id string, label ClaimLabel, v *ClaimResult) {
	if len(label.CompleteEvidenceGroups) == 0 {
		return
	}
	r.CompleteGroupEvaluated++
	if completeGoldGroup(label.CompleteEvidenceGroups, v.SupportingPassageIDs) {
		r.CompleteGroupCorrect++
		return
	}
	r.IncompleteEvidenceGroups++
	r.WorstMisses = append(r.WorstMisses, CalibrationMiss{CaseID: id, ClaimID: label.ClaimID, Kind: "incomplete_evidence_group", Strength: strength(v)})
}

func completeGoldGroup(groups [][]string, supporting []string) bool {
	for _, group := range groups {
		complete := true
		for _, id := range group {
			if !slices.Contains(supporting, id) {
				complete = false
				break
			}
		}
		if complete {
			return true
		}
	}
	return false
}

func compatibleRecordedReport(r *Report) bool {
	return r.Version == SchemaVersion && r.Rubric == RubricVersion && r.Policy == PolicyVersion
}
