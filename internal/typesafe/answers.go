package typesafe

import (
	"bytes"
	"encoding/json"
	"math"
	"strconv"
	"strings"

	"github.com/tphakala/jev-mcp/internal/evidence"
)

func (c *Client) decodeBatch(calls []evidence.Call, data []byte) []evidence.Answer {
	var parsed struct {
		Model   string                     `json:"model"`
		Answers map[string]json.RawMessage `json:"answers"`
	}
	if json.Unmarshal(data, &parsed) != nil || parsed.Answers == nil || len(parsed.Answers) > len(calls) {
		return invalidBatch(calls)
	}
	expected := make(map[string]bool, len(calls))
	for _, call := range calls {
		expected[call.ID] = true
	}
	for id := range parsed.Answers {
		if !expected[id] {
			return invalidBatch(calls)
		}
	}
	if parsed.Model != "" && (!safeIdentity(parsed.Model) || strings.Contains(parsed.Model, c.key)) {
		return invalidBatch(calls)
	}
	results := make([]evidence.Answer, 0, len(calls))
	for _, call := range calls {
		results = append(results, c.decodeAnswer(call, parsed.Answers[call.ID], parsed.Model))
	}
	return results
}

func (c *Client) decodeAnswer(call evidence.Call, raw json.RawMessage, resolved string) evidence.Answer {
	out := evidence.Answer{ID: call.ID, Provider: "typesafe", RequestedModel: c.model, ResolvedModel: resolved, Status: evidence.StatusFailed, Error: evidence.OutcomeInvalidProvider}
	var a wireAnswer
	if json.Unmarshal(raw, &a) != nil || a.Noul != nil || !unit(a.Confidence) || !distribution(a.Probabilities, call.Options) {
		return out
	}
	if call.Stage == evidence.StageSufficiency {
		level, valid := scoreResult(a, call.Options)
		if !valid {
			return out
		}
		out.SemanticLevel = level
		out.Score = a.Score
	} else {
		probability, valid := choiceResult(a, call.Options)
		if !valid {
			return out
		}
		out.Selected = a.Choice
		out.Probability = &probability
	}
	out.Confidence = a.Confidence
	out.Probabilities = a.Probabilities
	// Preserve primitive values and the actual legend. Exclude unknown diagnostic
	// fields that could contain reflected credentials or provider error text.
	primitive, err := json.Marshal(a)
	if err != nil || bytes.Contains(primitive, []byte(c.key)) {
		return evidence.Answer{ID: call.ID, Status: evidence.StatusFailed, Error: evidence.OutcomeInvalidProvider}
	}
	out.Raw = primitive
	out.Status = evidence.StatusSuccess
	out.Error = ""
	return out
}

func choiceResult(a wireAnswer, options map[string]string) (float64, bool) {
	if a.Type != "choice" || a.Score != nil || len(a.Legend) > 0 {
		return 0, false
	}
	if _, ok := options[a.Choice]; !ok {
		return 0, false
	}
	chosen := a.Probabilities[a.Choice]
	for _, p := range a.Probabilities {
		if p > chosen+1e-9 {
			return 0, false
		}
	}
	return chosen, true
}

func scoreResult(a wireAnswer, options map[string]string) (*int, bool) {
	levels, ok := scoreLevels(options)
	if !ok || a.Type != "score" || a.Choice != "" || a.Score == nil || *a.Score < 0 || *a.Score > float64(scoreLevelCount-1) || len(a.Legend) != scoreLevelCount {
		return nil, false
	}
	mean := 0.0
	highest := -1.0
	mode := -1
	tied := false
	for i, label := range levels {
		key := strconv.Itoa(i)
		if a.Legend[key] != label {
			return nil, false
		}
		p := a.Probabilities[key]
		mean += float64(i) * p
		if p > highest {
			highest = p
			mode = i
			tied = false
		} else if p == highest {
			tied = true
		}
	}
	if math.Abs(mean-*a.Score) > probabilityTolerance {
		return nil, false
	}
	if tied {
		return nil, true
	}
	return &mode, true
}
