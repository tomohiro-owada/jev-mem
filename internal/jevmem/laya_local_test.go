package jevmem

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLocalLayaEvaluatorProtocol(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test worker is a POSIX shell script")
	}
	dir := t.TempDir()
	worker := filepath.Join(dir, "fake-laya")
	script := `#!/bin/sh
echo '{"ready":true,"model":"test-model"}'
while IFS= read -r line; do
  echo '{"ok":true,"response":{"model":"laya-mlx:test-model","answers":{"risk":{"type":"score","score":2.5,"confidence":0.8}},"usage":{"input_tokens":12,"output_tokens":0}}}'
done
`
	if err := os.WriteFile(worker, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	evaluator := &LocalLayaEvaluator{Python: worker, Model: "test-model", Device: "cpu", BatchSize: 4}
	t.Cleanup(func() { _ = evaluator.Close() })
	response, err := evaluator.Evaluate(context.Background(), "state", map[string]JevQuestion{
		"risk": {Type: "score", Instructions: "risk", Criteria: []string{"low", "high"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Model != "laya-mlx:test-model" {
		t.Fatalf("model = %q", response.Model)
	}
	if response.Answers["risk"].Score == nil || *response.Answers["risk"].Score != 2.5 {
		t.Fatalf("answer = %#v", response.Answers["risk"])
	}
}

func TestResolveJevBackend(t *testing.T) {
	t.Setenv("JEV_BACKEND", "")
	for input, want := range map[string]string{
		"":         CloudJevBackend,
		"cloud":    CloudJevBackend,
		"jev":      CloudJevBackend,
		"local":    LocalLayaBackend,
		"laya-mlx": LocalLayaBackend,
	} {
		got, err := ResolveJevBackend("", input)
		if err != nil {
			t.Fatalf("ResolveJevBackend(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("ResolveJevBackend(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestLayaQuestionsWrapsDenseScoresAsCanonicalAnswers(t *testing.T) {
	questions := map[string]JevQuestion{
		"risk.applicability": {Type: "choice"},
		"risk.score":         {Type: "score"},
		"standalone":         {Type: "noul"},
	}
	local, dense := layaQuestions(questions)
	if _, ok := local["risk.applicability"]; ok {
		t.Fatal("applicability question was sent to Laya")
	}
	if len(dense) != 1 || dense[0] != "risk" {
		t.Fatalf("dense attributes = %#v", dense)
	}
	score, confidence := 2.5, 0.2
	response := JevResponse{Answers: map[string]JevAnswer{
		"risk.score": {Type: "score", Score: &score, Confidence: &confidence},
	}}
	wrapDenseLayaAnswers(&response, dense)
	answer := response.Answers["risk.applicability"]
	if answer.Choice != string(Applicable) || answer.Confidence == nil || *answer.Confidence != confidence {
		t.Fatalf("wrapped answer = %#v", answer)
	}
}
