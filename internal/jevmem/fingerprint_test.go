package jevmem

import "testing"

func fpValue(value, confidence float64) AttributeValue {
	return AttributeValue{Value: &value, Applicability: Applicable, Observation: Inferred, Confidence: confidence}
}

func TestEveryFingerprintAttributeHasAHighAnchor(t *testing.T) {
	if len(attributeHighAnchorsV1) != len(FingerprintAttributesV1) {
		t.Fatalf("got %d anchors for %d attributes", len(attributeHighAnchorsV1), len(FingerprintAttributesV1))
	}
	for _, definition := range FingerprintAttributesV1 {
		if attributeHighAnchorsV1[definition.ID] == "" {
			t.Errorf("missing high anchor for %s", definition.ID)
		}
	}
}

func TestFingerprintCatalogV1HasExactlyOneHundredStableAttributes(t *testing.T) {
	if len(FingerprintAttributesV1) != 100 {
		t.Fatalf("attribute count = %d", len(FingerprintAttributesV1))
	}
	seen := map[string]bool{}
	for index, def := range FingerprintAttributesV1 {
		if def.Index != index+1 {
			t.Fatalf("attribute %s index = %d, want %d", def.ID, def.Index, index+1)
		}
		if seen[def.ID] {
			t.Fatalf("duplicate attribute %s", def.ID)
		}
		seen[def.ID] = true
	}
}

func TestCompareFingerprintsSeparatesLayersAndReportsCoverage(t *testing.T) {
	left := Fingerprint{SchemaVersion: 1, Values: map[string]AttributeValue{
		"state_uncertainty":      fpValue(80, 1),
		"optionality_priority":   fpValue(90, 1),
		"hard_threshold_use":     fpValue(75, 1),
		"retained_option_extent": fpValue(80, 1),
		"success_measurability":  {Applicability: Unknown},
		"choice_exclusivity":     {Applicability: NotApplicable},
	}}
	right := Fingerprint{SchemaVersion: 1, Values: map[string]AttributeValue{
		"state_uncertainty":      fpValue(70, 1),
		"optionality_priority":   fpValue(85, 1),
		"hard_threshold_use":     fpValue(25, 1),
		"retained_option_extent": fpValue(70, 1),
		"success_measurability":  fpValue(50, 1),
	}}
	result, err := CompareFingerprints(left, right)
	if err != nil {
		t.Fatal(err)
	}
	if result.SharedAttributeCount != 4 {
		t.Fatalf("shared count = %d", result.SharedAttributeCount)
	}
	if len(result.Layers) != 4 {
		t.Fatalf("layers = %#v", result.Layers)
	}
	if result.Score <= 0 || result.Score > 1 {
		t.Fatalf("score = %f", result.Score)
	}
	if result.Coverage <= 0 || result.Coverage >= 1 {
		t.Fatalf("coverage = %f", result.Coverage)
	}
}

func TestValidateFingerprintRejectsValueForUnknown(t *testing.T) {
	value := 25.0
	err := ValidateFingerprint(Fingerprint{SchemaVersion: 1, Values: map[string]AttributeValue{
		"state_uncertainty": {Value: &value, Applicability: Unknown},
	}})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestCompareFingerprintsDiscountsMutuallySparseFingerprints(t *testing.T) {
	left := Fingerprint{SchemaVersion: 1, Values: map[string]AttributeValue{
		"optionality_priority": fpValue(100, 1),
	}}
	right := Fingerprint{SchemaVersion: 1, Values: map[string]AttributeValue{
		"optionality_priority": fpValue(100, 1),
	}}
	result, err := CompareFingerprints(left, right)
	if err != nil {
		t.Fatal(err)
	}
	if result.Score != 1 {
		t.Fatalf("raw similarity = %f, want 1", result.Score)
	}
	if result.Coverage != 0.01 {
		t.Fatalf("coverage = %f, want 0.01", result.Coverage)
	}
}
