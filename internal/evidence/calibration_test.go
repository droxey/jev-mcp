package evidence

import "testing"

func calibrationFixture() CalibrationSet {
	set := CalibrationSet{Version: "evidence-calibration-v1", Rubric: RubricVersion, Policy: PolicyVersion, Cases: []CalibrationCase{
		{ID: "false-support", Split: "heldout", LabelSource: "original human exercise label", Expected: []ClaimLabel{{ClaimID: "c", Level: 0, Contradiction: true}}, Report: Report{Outcome: OutcomeSelected, Claims: []ClaimResult{{ClaimID: "c", Complete: true, Answer: &Answer{Probabilities: map[string]float64{"0": 0.1, "1": 0.1, "2": 0.8}}}}}},
		{ID: "false-conflict", Split: "heldout", LabelSource: "original human exercise label", Expected: []ClaimLabel{{ClaimID: "c", Level: 2}}, ExpectedAbstain: true, Report: Report{Outcome: OutcomeAbstain, Claims: []ClaimResult{{ClaimID: "c", Conflict: true, ContradictingPassageIDs: []string{"p"}}}}},
		{ID: "training-excluded", Split: "train", LabelSource: "original human exercise label", Expected: []ClaimLabel{{ClaimID: "c", Level: 0}}, Report: Report{Outcome: OutcomeSelected, Claims: []ClaimResult{{ClaimID: "c", Complete: true}}}},
	}}
	for i := range set.Cases {
		set.Cases[i].Report.Version = SchemaVersion
		set.Cases[i].Report.Rubric = RubricVersion
		set.Cases[i].Report.Policy = PolicyVersion
		for j := range set.Cases[i].Report.Claims {
			n := 1
			if set.Cases[i].Report.Claims[j].Complete {
				n = 2
			}
			set.Cases[i].Report.Claims[j].Level = &n
		}
	}
	return set
}

func TestCalibrationHeldoutFailuresAndThresholdSweeps(t *testing.T) {
	r, err := Calibrate(calibrationFixture(), []float64{0.5, 0.9})
	if err != nil {
		t.Fatal(err)
	}
	if r.Cases != 2 || r.ExcludedTraining != 1 || r.FalseSupport != 1 || r.MissedContradictions != 1 || r.FalseConflicts != 1 || r.Abstentions != 1 || r.MissedSupport != 1 || len(r.WorstMisses) < 3 {
		t.Fatalf("%+v", r)
	}
	if r.Sweeps[0].FalseSupport != 1 || r.Sweeps[1].FalseSupport != 0 || r.Calibration != "advisory_not_a_calibrated_cutoff" {
		t.Fatalf("%+v", r)
	}
}

func TestCalibrationRejectsMissingLabelsAndInvalidThreshold(t *testing.T) {
	for _, change := range []func(*CalibrationSet){func(s *CalibrationSet) { s.Cases[0].LabelSource = "" }, func(s *CalibrationSet) { s.Cases[0].Expected[0].ClaimID = "missing" }, func(s *CalibrationSet) { s.Cases[0].Expected[0].Level = 3 }, func(s *CalibrationSet) { s.Cases[0].Split = "unexpected" }} {
		s := calibrationFixture()
		change(&s)
		if _, err := Calibrate(s, []float64{0.5}); err == nil {
			t.Fatal("accepted invalid labels")
		}
	}
	if _, err := Calibrate(calibrationFixture(), []float64{1.2}); err == nil {
		t.Fatal("accepted invalid threshold")
	}
}

func TestCalibrationRejectsStaleRecordedReportVersions(t *testing.T) {
	for _, change := range []func(*Report){func(r *Report) { r.Version = "v0" }, func(r *Report) { r.Rubric = "old-rubric" }, func(r *Report) { r.Policy = "old-policy" }} {
		set := calibrationFixture()
		change(&set.Cases[0].Report)
		if _, err := Calibrate(set, []float64{0.5}); err == nil {
			t.Fatal("stale recorded report accepted")
		}
	}
}

func TestCalibrationRequiresEveryMemberOfAnyGoldAlternativeGroup(t *testing.T) {
	for _, tc := range []struct {
		name    string
		ids     []string
		correct int
	}{{"partial", []string{"p"}, 0}, {"complete", []string{"p", "q"}, 1}, {"alternative", []string{"alternative"}, 1}} {
		t.Run(tc.name, func(t *testing.T) {
			set := calibrationFixture()
			set.Cases = set.Cases[:1]
			set.Cases[0].Expected = []ClaimLabel{{ClaimID: "c", Level: 2, CompleteEvidenceGroups: [][]string{{"p", "q"}, {"alternative"}}}}
			set.Cases[0].Report.Claims[0].SupportingPassageIDs = tc.ids
			r, err := Calibrate(set, []float64{0.5})
			if err != nil || r.CompleteGroupCorrect != tc.correct || r.CompleteGroupEvaluated != 1 || r.IncompleteEvidenceGroups != 1-tc.correct {
				t.Fatalf("%+v err=%v", r, err)
			}
		})
	}
}

func TestCalibrationSeparatesDocumentaryLevelFromPolicyAcceptance(t *testing.T) {
	set := calibrationFixture()
	set.Cases = set.Cases[:1]
	level := 2
	set.Cases[0].Expected = []ClaimLabel{{ClaimID: "c", Level: 2, Contradiction: true, Conflict: true}}
	set.Cases[0].Report.Claims[0] = ClaimResult{ClaimID: "c", Level: &level, Complete: false, Conflict: true, ContradictingPassageIDs: []string{"counter"}}
	r, err := Calibrate(set, []float64{0.5})
	if err != nil || r.MissedSupport != 0 || r.AcceptanceErrors != 0 {
		t.Fatalf("%+v err=%v", r, err)
	}
}
