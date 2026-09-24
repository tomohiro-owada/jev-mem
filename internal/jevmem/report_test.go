package jevmem

import (
	"strings"
	"testing"
	"time"
)

func TestRenderAnalogSearchHTMLOrdersHeatmapsByResults(t *testing.T) {
	query := completeTestFingerprint()
	first, second := completeTestFingerprint(), completeTestFingerprint()
	queryValue, firstValue, secondValue := 80.0, 75.0, 10.0
	query.Values["state_uncertainty"] = AttributeValue{Value: &queryValue, Applicability: Applicable, Confidence: 0.5}
	first.Values["state_uncertainty"] = AttributeValue{Value: &firstValue, Applicability: Applicable, Confidence: 0.5}
	second.Values["state_uncertainty"] = AttributeValue{Value: &secondValue, Applicability: Applicable, Confidence: 0.5}
	data := AnalogSearchData{QueryFingerprint: query, Results: []AnalogSearchResult{
		{Title: "First result <script>alert(1)</script>", Path: "first.md", CombinedScore: 0.9, FingerprintScore: 0.8, Fingerprint: &first},
		{Title: "Second result", Path: "second.md", CombinedScore: 0.7, FingerprintScore: 0.6, Fingerprint: &second},
	}}
	html, err := RenderAnalogSearchHTML(data, time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Index(html, "First result") >= strings.Index(html, "Second result") {
		t.Fatal("results are not rendered in input similarity order")
	}
	if count := strings.Count(html, `class="cell `); count != 500 {
		t.Fatalf("heatmap cell count = %d", count)
	}
	for _, expected := range []string{"記憶どうしの類似ヒートマップ", "現状の未把握度", "選択後に残した選択肢", "今回 <b>80</b> ↔ 過去 <b>75</b>", `class="cell match-9"`, "属性類似度"} {
		if !strings.Contains(html, expected) {
			t.Fatalf("report does not contain %q", expected)
		}
	}
	if strings.Contains(html, "<script>alert(1)</script>") {
		t.Fatal("result content was not HTML-escaped")
	}
}
