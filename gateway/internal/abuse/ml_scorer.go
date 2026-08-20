package abuse

import (
	"context"
	"fmt"
	"os"
)

// MLScorer implements the Rule interface using an in-process ONNX classifier.
type MLScorer struct {
	modelPath string
	threshold float64
	ensemble  *ONNXTreeEnsemble
	extractor *FeatureExtractor
}

// NewMLScorer initializes the ML scorer by loading the specified ONNX model.
// If the model path does not exist, an error is returned so caller can gracefully degrade.
func NewMLScorer(modelPath string, threshold float64) (*MLScorer, error) {
	if threshold <= 0.0 || threshold > 1.0 {
		threshold = 0.8
	}

	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("ONNX model file not found at %s: %w", modelPath, err)
	}

	ensemble, err := LoadONNXTreeEnsemble(modelPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ONNX model at %s: %w", modelPath, err)
	}

	return &MLScorer{
		modelPath: modelPath,
		threshold: threshold,
		ensemble:  ensemble,
		extractor: NewFeatureExtractor(),
	}, nil
}

// Name returns the rule identifier.
func (m *MLScorer) Name() string {
	return "ml_abuse_classifier"
}

// Evaluate extracts features from the incoming request and performs in-process ONNX inference.
func (m *MLScorer) Evaluate(ctx context.Context, req *RequestContext) (Decision, error) {
	if m.ensemble == nil {
		return Decision{
			Action: ActionAllow,
			Score:  0.0,
			Rule:   m.Name(),
			Reason: "model not loaded",
		}, nil
	}

	// Extract canonical 6-feature vector
	// At evaluate time, request status code is assumed 200 (pending upstream execution)
	features := m.extractor.Extract(req, 200)

	probs, err := m.ensemble.Predict(features)
	if err != nil {
		// Fail open on evaluation error
		return Decision{
			Action: ActionAllow,
			Score:  0.0,
			Rule:   m.Name(),
			Reason: fmt.Sprintf("inference error: %v", err),
		}, nil
	}

	abuseProb := float64(probs[1]) // P(abuse)

	if abuseProb >= m.threshold {
		return Decision{
			Action: ActionBlock,
			Score:  abuseProb,
			Rule:   m.Name(),
			Reason: fmt.Sprintf("ML abuse risk probability (%.2f) exceeds threshold (%.2f)", abuseProb, m.threshold),
		}, nil
	}

	if abuseProb >= 0.5 {
		return Decision{
			Action: ActionFlag,
			Score:  abuseProb,
			Rule:   m.Name(),
			Reason: fmt.Sprintf("ML elevated abuse risk probability (%.2f)", abuseProb),
		}, nil
	}

	return Decision{
		Action: ActionAllow,
		Score:  abuseProb,
		Rule:   m.Name(),
		Reason: fmt.Sprintf("ML normal traffic probability (%.2f)", float64(probs[0])),
	}, nil
}

// PredictRaw runs direct prediction on a given feature vector.
func (m *MLScorer) PredictRaw(features []float32) (float64, error) {
	if m.ensemble == nil {
		return 0.0, fmt.Errorf("model not loaded")
	}
	probs, err := m.ensemble.Predict(features)
	if err != nil {
		return 0.0, err
	}
	return float64(probs[1]), nil
}
