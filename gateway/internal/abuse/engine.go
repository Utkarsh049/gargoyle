package abuse

import (
	"context"
	"math"
)

// Engine evaluates incoming requests against registered heuristic rules
// and scoring models to determine an abuse decision.
type Engine struct {
	rules          []Rule
	blockThreshold float64
}

// NewEngine builds an Engine with the given rules and blocking score threshold.
func NewEngine(blockThreshold float64, rules ...Rule) *Engine {
	if blockThreshold <= 0.0 || blockThreshold > 1.0 {
		blockThreshold = 0.8
	}
	return &Engine{
		rules:          rules,
		blockThreshold: blockThreshold,
	}
}

// AddRule registers an additional heuristic rule or model into the engine.
func (e *Engine) AddRule(rule Rule) {
	if rule != nil {
		e.rules = append(e.rules, rule)
	}
}

// Evaluate runs all active heuristic rules against the request context and
// returns an aggregated decision.
//
// If any rule dictates an immediate block or produces a risk score exceeding
// blockThreshold, the request is blocked.
func (e *Engine) Evaluate(ctx context.Context, req *RequestContext) (Decision, error) {
	if len(e.rules) == 0 {
		return Decision{Action: ActionAllow, Score: 0.0}, nil
	}

	var maxScore float64
	var blockingDec Decision
	isBlocked := false

	for _, r := range e.rules {
		dec, err := r.Evaluate(ctx, req)
		if err != nil {
			// Rules fail open internally, but if an error propagates, continue
			continue
		}

		if dec.Score > maxScore {
			maxScore = math.Min(1.0, dec.Score)
		}

		if dec.Action == ActionBlock || dec.Score >= e.blockThreshold {
			if !isBlocked {
				isBlocked = true
				blockingDec = dec
				blockingDec.Action = ActionBlock
				if blockingDec.Score < e.blockThreshold {
					blockingDec.Score = e.blockThreshold
				}
			}
		}
	}

	if isBlocked {
		return blockingDec, nil
	}

	if maxScore >= 0.5 {
		return Decision{
			Action: ActionFlag,
			Score:  maxScore,
			Reason: "elevated abuse risk score",
		}, nil
	}

	return Decision{
		Action: ActionAllow,
		Score:  maxScore,
	}, nil
}
