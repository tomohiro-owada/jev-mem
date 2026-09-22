package jevmem

import (
	"strings"
	"testing"
	"time"
)

func TestRenderAndParseMemoryMarkdown(t *testing.T) {
	now := time.Date(2026, 6, 22, 1, 2, 3, 0, time.UTC)
	raw := RenderMemoryMarkdown("proj-123", "Decision", "Body\ntext", "test", now)
	if !strings.Contains(raw, "type: Memory") {
		t.Fatal("missing OKF type")
	}
	mem, ok := ParseMemoryMarkdown("projects/proj-123/file.md", raw)
	if !ok {
		t.Fatal("parse failed")
	}
	if mem.ProjectID != "proj-123" || mem.Title != "Decision" || mem.Content != "Body\ntext" {
		t.Fatalf("unexpected parse result: %+v", mem)
	}
}

func TestDecisionMarkdownRoundTripPreservesFingerprint(t *testing.T) {
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	fingerprint := Fingerprint{SchemaVersion: 1, Extractor: "jev", Values: map[string]AttributeValue{
		"state_uncertainty": fpValue(75, 0.8),
	}}
	raw, err := RenderDecisionMarkdown("proj", "Annual contract", DecisionInput{
		Statement:         "Do not sign the annual contract",
		Rationale:         "Preserve options while usage is uncertain",
		Evidence:          []string{"Monthly usage remains available"},
		Context:           "Annual discount expires soon",
		DecisionTime:      now.Format(time.RFC3339),
		FocalOption:       "Annual contract",
		ReferenceOption:   "Monthly plan",
		ChosenResponse:    "Continue monthly",
		EvaluationHorizon: "1 year",
	}, fingerprint, "cli", now)
	if err != nil {
		t.Fatal(err)
	}
	parsed, ok := ParseMemoryMarkdown("projects/proj/a.md", raw)
	if !ok {
		t.Fatal("failed to parse decision markdown")
	}
	if parsed.Fingerprint == nil || parsed.Fingerprint.SchemaVersion != 1 {
		t.Fatalf("fingerprint = %#v", parsed.Fingerprint)
	}
	if strings.Contains(parsed.Content, fingerprintStartMarker) {
		t.Fatal("fingerprint block leaked into semantic content")
	}
	if !strings.Contains(parsed.Content, "Preserve options") {
		t.Fatalf("content = %q", parsed.Content)
	}
}

func TestUniqueMemoryFilename(t *testing.T) {
	now := time.Date(2026, 6, 22, 1, 2, 3, 0, time.UTC)
	got := UniqueMemoryFilename("My Decision!", now, "a1b2c3")
	want := "my-decision_20260622_010203_a1b2c3.md"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
