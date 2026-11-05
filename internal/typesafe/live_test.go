package typesafe

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/tphakala/jev-mcp/internal/evidence"
)

// TestLiveProviderContract is an explicitly paid opt-in semantic smoke check,
// never calibration or proof of ChatGPT/iOS connectivity. It makes one request,
// one question, no retries, under a 45-second total deadline.
func TestLiveProviderContract(t *testing.T) {
	if os.Getenv("JEV_EVIDENCE_LIVE") != "1" {
		t.Skip("set JEV_EVIDENCE_LIVE=1 for one paid provider question")
	}
	c, err := New(Config{APIKey: os.Getenv("TYPESAFE_API_KEY"), Model: os.Getenv("JEV_EVIDENCE_MODEL")})
	if err != nil {
		t.Fatal("provider configuration incomplete")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	answers, err := c.Evaluate(ctx, []evidence.Call{{ID: "live-relation", Stage: evidence.StageRelation, State: json.RawMessage(`{"claim":"The example has a blue square.","passage":"This fictional example has a blue square."}`), Options: relationOptions()}})
	if err != nil {
		t.Fatal("live provider request failed; credentials and response text omitted")
	}
	if len(answers) != 1 || answers[0].Status != evidence.StatusSuccess {
		t.Fatal("live provider returned invalid contract")
	}
	if answers[0].Selected != evidence.Supports {
		t.Error("live semantic smoke case did not report support; no accuracy claim established")
	}
}
