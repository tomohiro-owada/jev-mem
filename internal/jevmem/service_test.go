package jevmem

import (
	"context"
	"testing"
)

type staticFingerprintExtractor struct {
	fingerprint Fingerprint
	err         error
}

func (e staticFingerprintExtractor) Extract(context.Context, DecisionInput) (Fingerprint, error) {
	return e.fingerprint, e.err
}

func TestValidateSaveRejectsOversizedContent(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Limits.MaxContentBytes = 5
	cfg.Limits.HardMaxContentBytes = 10
	svc := NewService(cfg, nil, HashEmbedder{})
	err := svc.validateSave(SaveRequest{CurrentWorkspacePath: "/tmp/proj", Title: "t", Content: "123456"})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestValidateSaveRequiresContent(t *testing.T) {
	svc := NewService(DefaultConfig(), nil, HashEmbedder{})
	err := svc.validateSave(SaveRequest{CurrentWorkspacePath: "/tmp/proj", Title: "t"})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestSaveDecisionDryRunProducesBothRepresentations(t *testing.T) {
	svc := NewService(DefaultConfig(), nil, HashEmbedder{}).WithFingerprintExtractor(staticFingerprintExtractor{fingerprint: Fingerprint{
		SchemaVersion: 1,
		Extractor:     "test",
		Values: map[string]AttributeValue{
			"optionality_priority": fpValue(90, 1),
		},
	}})
	response := svc.SaveDecision(context.Background(), SaveDecisionRequest{
		CurrentWorkspacePath: "/tmp/project",
		Title:                "Delay annual contract",
		Decision: DecisionInput{
			Statement:      "Delay the annual contract",
			Rationale:      "Keep options open while evidence is incomplete",
			FocalOption:    "Sign now",
			ChosenResponse: "Delay",
		},
		DryRun: true,
	})
	if !response.OK {
		t.Fatalf("unexpected failure: %+v", response.Error)
	}
	if !response.Data.DryRun || !response.Data.Fingerprinted || response.Data.EmbeddingDim == 0 {
		t.Fatalf("unexpected result: %+v", response.Data)
	}
}
