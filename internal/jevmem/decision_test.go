package jevmem

import (
	"context"
	"strings"
	"testing"
)

type fakeJevEvaluator struct{}

func (fakeJevEvaluator) Evaluate(_ context.Context, _ any, questions map[string]JevQuestion) (JevResponse, error) {
	answers := make(map[string]JevAnswer, len(questions))
	confidence := 0.8
	score := 3.0
	for id, question := range questions {
		switch question.Type {
		case "choice":
			choice := "applicable"
			if strings.HasPrefix(id, "risk_coupling.") {
				choice = "not_applicable"
			}
			answers[id] = JevAnswer{Type: "choice", Choice: choice, Confidence: &confidence}
		case "score":
			answers[id] = JevAnswer{Type: "score", Score: &score, Confidence: &confidence}
		}
	}
	return JevResponse{Model: "jev-test", Answers: answers}, nil
}

func TestJevFingerprintExtractorBuildsOneHundredAttributes(t *testing.T) {
	extractor := &JevFingerprintExtractor{Evaluator: fakeJevEvaluator{}, BatchSize: 17}
	fingerprint, err := extractor.Extract(context.Background(), DecisionInput{
		Statement:       "年間契約を見送る",
		Rationale:       "利用量が不明なため選択肢を残す",
		Evidence:        []string{"月額契約で試用可能"},
		Context:         "割引期限はあるが月額利用を継続できる",
		FocalOption:     "SaaS年間契約",
		ReferenceOption: "月額契約",
		ChosenResponse:  "年間契約を見送り、月額で計測する",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(fingerprint.Values) != 100 {
		t.Fatalf("attribute count = %d", len(fingerprint.Values))
	}
	if fingerprint.ExtractorVersion != "jev-test" {
		t.Fatalf("extractor version = %q", fingerprint.ExtractorVersion)
	}
	if fingerprint.Values["risk_coupling"].Applicability != NotApplicable {
		t.Fatalf("risk_coupling = %#v", fingerprint.Values["risk_coupling"])
	}
	value := fingerprint.Values["optionality_priority"]
	if value.Value == nil || *value.Value != 75 {
		t.Fatalf("optionality_priority = %#v", value)
	}
}
