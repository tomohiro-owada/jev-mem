package jevmem

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var markdownHeadingRE = regexp.MustCompile(`(?m)^##[ \t]+(.+?)[ \t]*$`)

// ParseLegacyDecision converts the Japanese gmem-memory section format into
// the explicit decision frame required by the fingerprint extractor.
func ParseLegacyDecision(path, raw string) (DecisionInput, bool) {
	matches := frontMatterRE.FindStringSubmatch(raw)
	if len(matches) != 3 {
		return DecisionInput{}, false
	}
	sections := legacySections(matches[2])
	statement := firstLegacySection(sections, "決定内容")
	if strings.TrimSpace(statement) == "" {
		return DecisionInput{}, false
	}
	rationale := firstLegacySection(sections, "決定基準・理由")
	if strings.TrimSpace(rationale) == "" {
		// Dense local extraction still needs a rationale field. Preserve the
		// record rather than inventing one; confidence carries model uncertainty.
		rationale = statement
	}
	context := firstLegacySection(sections, "場面")
	reference := firstLegacySection(sections, "選択肢")
	date := firstLegacySection(sections, "日付")
	date = strings.TrimSpace(strings.Split(date, "\n")[0])
	return DecisionInput{
		Statement:       strings.TrimSpace(statement),
		Rationale:       strings.TrimSpace(rationale),
		Context:         strings.TrimSpace(context),
		DecisionTime:    date,
		FocalOption:     strings.TrimSpace(statement),
		ReferenceOption: strings.TrimSpace(reference),
		ChosenResponse:  strings.TrimSpace(statement),
	}, true
}

func legacySections(body string) map[string]string {
	matches := markdownHeadingRE.FindAllStringSubmatchIndex(body, -1)
	sections := map[string]string{}
	for i, match := range matches {
		heading := strings.TrimSpace(body[match[2]:match[3]])
		start := match[1]
		end := len(body)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		sections[heading] = strings.TrimSpace(body[start:end])
	}
	return sections
}

func firstLegacySection(sections map[string]string, prefix string) string {
	if value := sections[prefix]; value != "" {
		return value
	}
	keys := make([]string, 0, len(sections))
	for key := range sections {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if strings.HasPrefix(key, prefix) {
			return sections[key]
		}
	}
	return ""
}

// AttachFingerprint preserves the legacy document and appends the canonical
// embedded block used by ParseMemoryMarkdown and index rebuilds.
func AttachFingerprint(raw string, fingerprint Fingerprint) (string, error) {
	if err := ValidateFingerprint(fingerprint); err != nil {
		return "", err
	}
	if _, existing := splitFingerprint(raw); existing != nil {
		return raw, nil
	}
	encoded, err := json.MarshalIndent(fingerprint, "", "  ")
	if err != nil {
		return "", err
	}
	matches := frontMatterRE.FindStringSubmatch(raw)
	if len(matches) != 3 {
		return "", fmt.Errorf("invalid memory Markdown")
	}
	meta := matches[1]
	if !strings.Contains(meta, "\nfingerprint_schema:") && !strings.HasPrefix(meta, "fingerprint_schema:") {
		meta += fmt.Sprintf("\nfingerprint_schema: %d", fingerprint.SchemaVersion)
	}
	body := strings.TrimRight(matches[2], "\n")
	return "---\n" + meta + "\n---\n\n" + body + "\n\n" +
		fingerprintStartMarker + "\n```json\n" + string(encoded) + "\n```\n" + fingerprintEndMarker + "\n", nil
}
