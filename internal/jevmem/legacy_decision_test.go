package jevmem

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

const legacyDecisionFixture = `---
type: Memory
title: "年間契約を延期"
timestamp: 2026-08-08T22:53:46Z
project_id: "sample"
source: "mcp"
---

## 場面
要件が変化中である。

## 決定内容
年間契約を延期して月次試用を行う。

## 選択肢
- 年間契約
- 月次試用

## 決定基準・理由
情報不足で長期拘束を避けたかった。

## 日付
2026-08-08
`

func TestParseLegacyDecision(t *testing.T) {
	decision, ok := ParseLegacyDecision("sample.md", legacyDecisionFixture)
	if !ok {
		t.Fatal("legacy decision was not detected")
	}
	if decision.Statement != "年間契約を延期して月次試用を行う。" {
		t.Fatalf("statement = %q", decision.Statement)
	}
	if decision.DecisionTime != "2026-08-08" {
		t.Fatalf("decision time = %q", decision.DecisionTime)
	}
	if decision.ReferenceOption == "" || decision.Rationale == "" || decision.Context == "" {
		t.Fatalf("decision = %#v", decision)
	}
}

func TestAttachFingerprintRoundTrip(t *testing.T) {
	fingerprint := completeTestFingerprint()
	updated, err := AttachFingerprint(legacyDecisionFixture, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	memory, ok := ParseMemoryMarkdown("sample.md", updated)
	if !ok || memory.Fingerprint == nil {
		t.Fatal("attached fingerprint was not parsed")
	}
	if len(memory.Fingerprint.Values) != 100 {
		t.Fatalf("values = %d", len(memory.Fingerprint.Values))
	}
	second, err := AttachFingerprint(updated, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	if second != updated {
		t.Fatal("attachment is not idempotent")
	}
}

func TestBackfillLegacyFingerprintsIsResumable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "projects", "sample", "decision.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(legacyDecisionFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	extractor := &JevFingerprintExtractor{Evaluator: fakeJevEvaluator{}, BatchSize: 25}
	first, err := BackfillLegacyFingerprints(context.Background(), extractor, BackfillOptions{RepoDir: dir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.Updated != 1 || first.Failed != 0 {
		t.Fatalf("first = %#v", first)
	}
	second, err := BackfillLegacyFingerprints(context.Background(), extractor, BackfillOptions{RepoDir: dir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if second.Updated != 0 || second.Skipped != 1 {
		t.Fatalf("second = %#v", second)
	}
}

func completeTestFingerprint() Fingerprint {
	value := 50.0
	values := make(map[string]AttributeValue, len(FingerprintAttributesV1))
	for _, definition := range FingerprintAttributesV1 {
		values[definition.ID] = AttributeValue{Value: &value, Applicability: Applicable, Observation: Inferred, Confidence: 0.5}
	}
	return Fingerprint{SchemaVersion: FingerprintSchemaV1, Extractor: "test", Values: values}
}
