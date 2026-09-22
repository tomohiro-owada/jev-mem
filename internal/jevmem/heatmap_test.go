package jevmem

import (
	"strings"
	"testing"
)

func TestRenderFingerprintSVGHasOneCellPerStableAttribute(t *testing.T) {
	fingerprint := Fingerprint{SchemaVersion: 1, Values: map[string]AttributeValue{
		"state_uncertainty":      fpValue(75, 1),
		"optionality_priority":   {Applicability: Unknown},
		"retained_option_extent": {Applicability: NotApplicable},
	}}
	svg, err := RenderFingerprintSVG(fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(svg, "<g>") != 100 {
		t.Fatalf("cell count = %d", strings.Count(svg, "<g>"))
	}
	for _, expected := range []string{"01 現状の未把握度", "100 選択後に残した選択肢", "Decision fingerprint heatmap"} {
		if !strings.Contains(svg, expected) {
			t.Fatalf("SVG does not contain %q", expected)
		}
	}
}
