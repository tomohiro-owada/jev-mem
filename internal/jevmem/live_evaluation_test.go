package jevmem

import (
	"context"
	"os"
	"testing"
)

// TestLiveJevCrossDomainAnalogy is an opt-in acceptance evaluation. It uses the
// real Jev API and is intentionally skipped in CI unless JEV_LIVE_TEST=1.
func TestLiveJevCrossDomainAnalogy(t *testing.T) {
	if os.Getenv("JEV_LIVE_TEST") != "1" {
		t.Skip("set JEV_LIVE_TEST=1 to run the live Jev acceptance evaluation")
	}
	client, err := NewJevClientFromEnvironment("../..")
	if err != nil {
		t.Fatal(err)
	}
	extractor := &JevFingerprintExtractor{Evaluator: client, BatchSize: 25}
	cases := map[string]DecisionInput{
		"database_delay": {
			Statement:   "Postpone the full migration to a proprietary database",
			Rationale:   "Requirements and migration behavior remain uncertain. There is no deadline requiring an irreversible move now, so retain the current database and run a representative migration trial before committing.",
			Evidence:    []string{"Only a small synthetic benchmark has been run", "A reversible pilot can test production-like workloads"},
			Context:     "A full migration would create substantial switching cost and constrain later architecture choices for several years.",
			FocalOption: "Migrate every workload now", ReferenceOption: "Keep the current database during a pilot", ChosenResponse: "Postpone full migration and run a pilot", EvaluationHorizon: "3 years",
		},
		"saas_delay": {
			Statement:   "Do not sign the annual SaaS contract yet",
			Rationale:   "Requirements and product fit remain uncertain. There is no deadline requiring a long commitment now, so preserve alternatives and use a monthly trial to gather evidence before committing.",
			Evidence:    []string{"Only a vendor demonstration has been completed", "A monthly trial is available"},
			Context:     "The annual plan is cheaper but creates a twelve-month commitment and switching work while requirements are changing.",
			FocalOption: "Sign the annual contract now", ReferenceOption: "Use a monthly trial", ChosenResponse: "Delay annual commitment and run a trial", EvaluationHorizon: "12 months",
		},
		"equipment_replace": {
			Statement:   "Replace the repairable production machine immediately",
			Rationale:   "Reliable uninterrupted output is the hard requirement. Repeated failures already cross the accepted downtime threshold, so accept the upfront cost and replace the machine now.",
			Evidence:    []string{"Three documented failures occurred this quarter", "Replacement capacity and delivery date are confirmed"},
			Context:     "The existing machine can be repaired again at low cost, while replacement is expensive but has a proven reliability record.",
			FocalOption: "Replace the machine now", ReferenceOption: "Repair it again", ChosenResponse: "Commit to immediate full replacement", EvaluationHorizon: "5 years",
		},
	}
	fingerprints := map[string]Fingerprint{}
	for name, decision := range cases {
		fingerprint, err := extractor.Extract(context.Background(), decision)
		if err != nil {
			t.Fatalf("extract %s: %v", name, err)
		}
		fingerprints[name] = fingerprint
	}
	structural, err := CompareFingerprints(fingerprints["database_delay"], fingerprints["saas_delay"])
	if err != nil {
		t.Fatal(err)
	}
	unrelated, err := CompareFingerprints(fingerprints["database_delay"], fingerprints["equipment_replace"])
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("database↔saas score=%.3f coverage=%.3f confidence=%.3f", structural.Score, structural.Coverage, structural.EffectiveConfidence)
	t.Logf("database↔equipment score=%.3f coverage=%.3f confidence=%.3f", unrelated.Score, unrelated.Coverage, unrelated.EffectiveConfidence)
	if structural.Score <= unrelated.Score {
		t.Fatalf("expected cross-domain analogy score %.3f to exceed unrelated score %.3f", structural.Score, unrelated.Score)
	}
}
