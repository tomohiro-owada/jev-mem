package jevmem

import (
	"strings"
	"testing"
	"time"
)

func TestRenderAnalogSearchHTMLOrdersHeatmapsByResults(t *testing.T) {
	query := completeTestFingerprint()
	first, second := completeTestFingerprint(), completeTestFingerprint()
	data := AnalogSearchData{QueryFingerprint: query, Results: []AnalogSearchResult{
		{Title: "First result", Path: "first.md", CombinedScore: 0.9, FingerprintScore: 0.8, Fingerprint: &first},
		{Title: "Second result", Path: "second.md", CombinedScore: 0.7, FingerprintScore: 0.6, Fingerprint: &second},
	}}
	html, err := RenderAnalogSearchHTML(data, time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Index(html, "First result") >= strings.Index(html, "Second result") {
		t.Fatal("results are not rendered in input similarity order")
	}
	if count := strings.Count(html, `class="cell `); count != 300 {
		t.Fatalf("heatmap cell count = %d", count)
	}
	for _, expected := range []string{"Decision Fingerprint Search", "01 現状の未把握度", "100 選択後に残した選択肢"} {
		if !strings.Contains(html, expected) {
			t.Fatalf("report does not contain %q", expected)
		}
	}
}
