package jevmem

import (
	"bytes"
	"fmt"
	"html"
)

// RenderFingerprintSVG renders the stable 100 attributes as a human-facing
// 10x10 heatmap. Search always uses Fingerprint.Values, never these pixels.
func RenderFingerprintSVG(fingerprint Fingerprint) (string, error) {
	if err := ValidateFingerprint(fingerprint); err != nil {
		return "", err
	}
	const cell, gap, margin = 44, 4, 12
	size := margin*2 + cell*10 + gap*9
	var b bytes.Buffer
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img" aria-label="Decision fingerprint heatmap">`, size, size, size, size)
	fmt.Fprint(&b, `<rect width="100%" height="100%" rx="12" fill="#0f172a"/>`)
	for index, definition := range FingerprintAttributesV1 {
		row, column := index/10, index%10
		x, y := margin+column*(cell+gap), margin+row*(cell+gap)
		value, exists := fingerprint.Values[definition.ID]
		fill, label := "#334155", "unknown"
		if exists {
			switch value.Applicability {
			case Applicable:
				fill = heatColor(*value.Value)
				label = fmt.Sprintf("%.0f", *value.Value)
			case NotApplicable:
				fill, label = "#111827", "not applicable"
			}
		}
		fmt.Fprintf(&b, `<g><title>%02d %s (%s): %s</title><rect x="%d" y="%d" width="%d" height="%d" rx="5" fill="%s"/>`, definition.Index, html.EscapeString(definition.NameJA), html.EscapeString(definition.ID), label, x, y, cell, cell, fill)
		fmt.Fprintf(&b, `<text x="%d" y="%d" text-anchor="middle" dominant-baseline="middle" fill="#f8fafc" font-family="system-ui,sans-serif" font-size="11">%02d</text></g>`, x+cell/2, y+cell/2, definition.Index)
	}
	fmt.Fprint(&b, `</svg>`)
	return b.String(), nil
}

func heatColor(value float64) string {
	// A single-hue lightness ramp keeps the ordinal meaning clear.
	lightness := 78 - value*0.5
	return fmt.Sprintf("hsl(198 88%% %.1f%%)", lightness)
}
