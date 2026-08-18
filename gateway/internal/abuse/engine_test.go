package abuse

import (
	"context"
	"testing"
	"time"
)

type mockRule struct {
	name     string
	evalFunc func(ctx context.Context, req *RequestContext) (Decision, error)
}

func (m *mockRule) Name() string {
	return m.name
}

func (m *mockRule) Evaluate(ctx context.Context, req *RequestContext) (Decision, error) {
	return m.evalFunc(ctx, req)
}

func TestEngineEvaluateClean(t *testing.T) {
	r1 := &mockRule{
		name: "clean_rule",
		evalFunc: func(_ context.Context, _ *RequestContext) (Decision, error) {
			return Decision{Action: ActionAllow, Score: 0.0, Rule: "clean_rule"}, nil
		},
	}

	engine := NewEngine(0.8, r1)
	req := &RequestContext{
		ClientID:  "client-1",
		IP:        "192.0.2.1",
		Path:      "/api/v1/resource",
		Timestamp: time.Now(),
	}

	dec, err := engine.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Action != ActionAllow {
		t.Fatalf("expected ActionAllow, got %s", dec.Action)
	}
	if dec.Score != 0.0 {
		t.Fatalf("expected score 0.0, got %v", dec.Score)
	}
}

func TestEngineEvaluateBlockingRule(t *testing.T) {
	r1 := &mockRule{
		name: "clean_rule",
		evalFunc: func(_ context.Context, _ *RequestContext) (Decision, error) {
			return Decision{Action: ActionAllow, Score: 0.1, Rule: "clean_rule"}, nil
		},
	}

	r2 := &mockRule{
		name: "bad_bot_rule",
		evalFunc: func(_ context.Context, _ *RequestContext) (Decision, error) {
			return Decision{
				Action: ActionBlock,
				Score:  0.95,
				Rule:   "bad_bot_rule",
				Reason: "malicious scraping pattern detected",
			}, nil
		},
	}

	engine := NewEngine(0.8, r1, r2)
	req := &RequestContext{
		ClientID:  "client-1",
		IP:        "192.0.2.1",
		Path:      "/api/v1/scrape",
		Timestamp: time.Now(),
	}

	dec, err := engine.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Action != ActionBlock {
		t.Fatalf("expected ActionBlock, got %s", dec.Action)
	}
	if dec.Rule != "bad_bot_rule" {
		t.Fatalf("expected Rule bad_bot_rule, got %s", dec.Rule)
	}
	if dec.Score < 0.8 {
		t.Fatalf("expected score >= 0.8, got %v", dec.Score)
	}
}

func TestEngineEvaluateScoreExceedingThreshold(t *testing.T) {
	r1 := &mockRule{
		name: "high_risk_rule",
		evalFunc: func(_ context.Context, _ *RequestContext) (Decision, error) {
			// Rule returned ActionAllow/ActionFlag but high risk score
			return Decision{Action: ActionFlag, Score: 0.85, Rule: "high_risk_rule", Reason: "high risk score"}, nil
		},
	}

	engine := NewEngine(0.8, r1)
	req := &RequestContext{
		ClientID: "client-1",
		IP:       "192.0.2.1",
		Path:     "/api/v1/test",
	}

	dec, err := engine.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Action != ActionBlock {
		t.Fatalf("expected score 0.85 to trigger ActionBlock against threshold 0.8, got %s", dec.Action)
	}
}

func TestEngineAddRule(t *testing.T) {
	engine := NewEngine(0.8)
	req := &RequestContext{
		ClientID: "client-1",
		IP:       "192.0.2.1",
		Path:     "/api/v1/test",
	}

	dec, err := engine.Evaluate(context.Background(), req)
	if err != nil || dec.Action != ActionAllow {
		t.Fatalf("expected allow for empty engine, got %s", dec.Action)
	}

	engine.AddRule(&mockRule{
		name: "added_rule",
		evalFunc: func(_ context.Context, _ *RequestContext) (Decision, error) {
			return Decision{Action: ActionBlock, Score: 0.9, Rule: "added_rule", Reason: "dynamic rule fired"}, nil
		},
	})

	dec2, err := engine.Evaluate(context.Background(), req)
	if err != nil || dec2.Action != ActionBlock {
		t.Fatalf("expected block after adding rule, got %s", dec2.Action)
	}
}
