package abuse

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func findModelPath() string {
	candidates := []string{
		"../../../abuse_model.onnx",
		"../../abuse_model.onnx",
		"../abuse_model.onnx",
		"abuse_model.onnx",
		"/home/utkarsh/Desktop/gargoyle/abuse_model.onnx",
	}
	for _, c := range candidates {
		if abs, err := filepath.Abs(c); err == nil {
			if _, err := os.Stat(abs); err == nil {
				return abs
			}
		}
	}
	return ""
}

func TestMLScorer_MissingModel(t *testing.T) {
	_, err := NewMLScorer("non_existent_model.onnx", 0.8)
	if err == nil {
		t.Fatal("expected error when initializing with non-existent model file, got nil")
	}
}

func TestMLScorer_InferenceParity(t *testing.T) {
	modelPath := findModelPath()
	if modelPath == "" {
		t.Skip("abuse_model.onnx not found, skipping inference test")
	}

	scorer, err := NewMLScorer(modelPath, 0.8)
	if err != nil {
		t.Fatalf("failed to load ONNX model: %v", err)
	}

	t.Logf("trees: %d, nodes: %d, classweights: %d, modes: %d", scorer.ensemble.NumTrees, len(scorer.ensemble.NodesTreeIDs), len(scorer.ensemble.ClassWeights), len(scorer.ensemble.NodesModes))

	// 1. Cold start vector: [1.0, 0.0, 0.0, 1.0, 0.0, 0.0]
	coldStartScore, err := scorer.PredictRaw([]float32{1.0, 0.0, 0.0, 1.0, 0.0, 0.0})
	if err != nil {
		t.Fatalf("PredictRaw failed on cold start: %v", err)
	}
	// Python onnxruntime returned: 0.17
	if coldStartScore < 0.10 || coldStartScore > 0.25 {
		t.Errorf("expected cold start abuse score around 0.17, got %f", coldStartScore)
	}

	// 2. Clear attack vector: [4.0, 100.0, 0.0, 4.0, 4.0, 0.8]
	attackScore, err := scorer.PredictRaw([]float32{4.0, 100.0, 0.0, 4.0, 4.0, 0.8})
	if err != nil {
		t.Fatalf("PredictRaw failed on attack vector: %v", err)
	}
	// Python onnxruntime returned: ~0.999
	if attackScore < 0.90 {
		t.Errorf("expected attack score > 0.90, got %f", attackScore)
	}
}

func TestMLScorer_Evaluate(t *testing.T) {
	modelPath := findModelPath()
	if modelPath == "" {
		t.Skip("abuse_model.onnx not found, skipping evaluate test")
	}

	scorer, err := NewMLScorer(modelPath, 0.8)
	if err != nil {
		t.Fatalf("failed to load ONNX model: %v", err)
	}

	ctx := context.Background()

	// Clean request: Should return ActionAllow
	cleanReq := &RequestContext{
		ClientID:  "test-user-clean",
		Path:      "/api/v1/products",
		Timestamp: time.Now().UTC(),
		Header: http.Header{
			"User-Agent":      []string{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"},
			"Accept":          []string{"application/json"},
			"Accept-Language": []string{"en-US,en;q=0.9"},
		},
		UserAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36",
	}

	dec, err := scorer.Evaluate(ctx, cleanReq)
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	if dec.Action != ActionAllow {
		t.Errorf("expected clean request to have ActionAllow, got %s (score: %f)", dec.Action, dec.Score)
	}

	// Scanner tool request: Should trigger ActionBlock or ActionFlag
	scannerReq := &RequestContext{
		ClientID:  "test-attacker-sqlmap",
		Path:      "/admin",
		Timestamp: time.Now().UTC(),
		Header: http.Header{
			"User-Agent": []string{"sqlmap/1.7#stable"},
			"Accept":     []string{"*/*"},
		},
		UserAgent: "sqlmap/1.7#stable",
	}

	// Fire repeated requests to establish history
	for i := 0; i < 4; i++ {
		scannerReq.Timestamp = time.Now().UTC().Add(time.Duration(i*50) * time.Millisecond)
		dec, err = scorer.Evaluate(ctx, scannerReq)
		if err != nil {
			t.Fatalf("Evaluate failed on iteration %d: %v", i, err)
		}
	}

	if dec.Score < 0.5 {
		t.Errorf("expected attack score >= 0.5, got %f", dec.Score)
	}
}
