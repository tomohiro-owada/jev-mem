package jevmem

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	fingerprintStartMarker = "<!-- jev-mem:fingerprint:start -->"
	fingerprintEndMarker   = "<!-- jev-mem:fingerprint:end -->"
)

func RenderMemoryMarkdown(projectID, title, content, source string, createdAt time.Time) string {
	var b bytes.Buffer
	fmt.Fprintln(&b, "---")
	fmt.Fprintln(&b, "type: Memory")
	fmt.Fprintf(&b, "title: %s\n", yamlScalar(title))
	fmt.Fprintf(&b, "description: %s\n", yamlScalar(firstLine(content)))
	fmt.Fprintln(&b, "resource: null")
	fmt.Fprintln(&b, "tags: []")
	fmt.Fprintf(&b, "timestamp: %s\n", createdAt.UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "project_id: %s\n", yamlScalar(projectID))
	fmt.Fprintf(&b, "source: %s\n", yamlScalar(source))
	fmt.Fprintln(&b, "---")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, content)
	return b.String()
}

func RenderDecisionMarkdown(projectID, title string, decision DecisionInput, fingerprint Fingerprint, source string, createdAt time.Time) (string, error) {
	if err := decision.Validate(); err != nil {
		return "", err
	}
	if err := ValidateFingerprint(fingerprint); err != nil {
		return "", err
	}
	fingerprintJSON, err := json.MarshalIndent(fingerprint, "", "  ")
	if err != nil {
		return "", err
	}
	var content bytes.Buffer
	fmt.Fprintln(&content, "# Decision")
	fmt.Fprintln(&content)
	fmt.Fprintln(&content, decision.Statement)
	fmt.Fprintln(&content)
	fmt.Fprintln(&content, "## Rationale")
	fmt.Fprintln(&content)
	fmt.Fprintln(&content, decision.Rationale)
	fmt.Fprintln(&content)
	fmt.Fprintln(&content, "## Evidence")
	fmt.Fprintln(&content)
	if len(decision.Evidence) == 0 {
		fmt.Fprintln(&content, "- None recorded")
	} else {
		for _, evidence := range decision.Evidence {
			fmt.Fprintf(&content, "- %s\n", strings.TrimSpace(evidence))
		}
	}
	fmt.Fprintln(&content)
	fmt.Fprintln(&content, "## Context")
	fmt.Fprintln(&content)
	fmt.Fprintln(&content, decision.Context)
	fmt.Fprintln(&content)
	fmt.Fprintln(&content, "## Decision Frame")
	fmt.Fprintln(&content)
	fmt.Fprintf(&content, "- Decision time: %s\n", decision.DecisionTime)
	fmt.Fprintf(&content, "- Focal option: %s\n", decision.FocalOption)
	fmt.Fprintf(&content, "- Reference option: %s\n", decision.ReferenceOption)
	fmt.Fprintf(&content, "- Chosen response: %s\n", decision.ChosenResponse)
	fmt.Fprintf(&content, "- Evaluation horizon: %s\n", decision.EvaluationHorizon)
	fmt.Fprintln(&content)
	fmt.Fprintln(&content, fingerprintStartMarker)
	fmt.Fprintln(&content, "```json")
	fmt.Fprintln(&content, string(fingerprintJSON))
	fmt.Fprintln(&content, "```")
	fmt.Fprintln(&content, fingerprintEndMarker)

	var result bytes.Buffer
	fmt.Fprintln(&result, "---")
	fmt.Fprintln(&result, "type: Decision")
	fmt.Fprintf(&result, "title: %s\n", yamlScalar(title))
	fmt.Fprintf(&result, "description: %s\n", yamlScalar(firstLine(decision.Statement)))
	fmt.Fprintln(&result, "resource: null")
	fmt.Fprintln(&result, "tags: []")
	fmt.Fprintf(&result, "timestamp: %s\n", createdAt.UTC().Format(time.RFC3339))
	fmt.Fprintf(&result, "project_id: %s\n", yamlScalar(projectID))
	fmt.Fprintf(&result, "source: %s\n", yamlScalar(source))
	fmt.Fprintf(&result, "fingerprint_schema: %d\n", fingerprint.SchemaVersion)
	fmt.Fprintln(&result, "---")
	fmt.Fprintln(&result)
	result.Write(content.Bytes())
	return result.String(), nil
}

func ContentHash(title, content string) string {
	sum := sha256.Sum256([]byte(title + "\n\n" + content))
	return hex.EncodeToString(sum[:])
}

func UniqueMemoryFilename(title string, now time.Time, randomSuffix string) string {
	prefix := sanitizeSegment(strings.ToLower(title))
	if prefix == "" {
		prefix = "memory"
	}
	if len(prefix) > 32 {
		prefix = prefix[:32]
		prefix = strings.Trim(prefix, "-.")
	}
	return fmt.Sprintf("%s_%s_%s.md", prefix, now.UTC().Format("20060102_150405"), randomSuffix)
}

var frontMatterRE = regexp.MustCompile(`(?s)^---\n(.*?)\n---\n\n?(.*)$`)

func ParseMemoryMarkdown(path, raw string) (Memory, bool) {
	matches := frontMatterRE.FindStringSubmatch(raw)
	if len(matches) != 3 {
		return Memory{}, false
	}
	meta, body := matches[1], matches[2]
	content, fingerprint := splitFingerprint(body)
	mem := Memory{Path: path, Content: strings.TrimRight(content, "\n"), Fingerprint: fingerprint}
	for _, line := range strings.Split(meta, "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key := strings.TrimSpace(k)
		val := strings.Trim(strings.TrimSpace(v), `"`)
		switch key {
		case "title":
			mem.Title = val
		case "project_id":
			mem.ProjectID = val
		case "timestamp":
			if t, err := time.Parse(time.RFC3339, val); err == nil {
				mem.CreatedAt = t
			}
		}
	}
	mem.Hash = ContentHash(mem.Title, strings.TrimRight(body, "\n"))
	return mem, true
}

func splitFingerprint(body string) (string, *Fingerprint) {
	start := strings.Index(body, fingerprintStartMarker)
	if start < 0 {
		return body, nil
	}
	endRelative := strings.Index(body[start:], fingerprintEndMarker)
	if endRelative < 0 {
		return body, nil
	}
	end := start + endRelative + len(fingerprintEndMarker)
	block := body[start+len(fingerprintStartMarker) : start+endRelative]
	block = strings.TrimSpace(block)
	block = strings.TrimPrefix(block, "```json")
	block = strings.TrimSuffix(block, "```")
	block = strings.TrimSpace(block)
	var fingerprint Fingerprint
	if err := json.Unmarshal([]byte(block), &fingerprint); err != nil {
		return body, nil
	}
	if err := ValidateFingerprint(fingerprint); err != nil {
		return body, nil
	}
	content := strings.TrimRight(body[:start], "\n") + body[end:]
	return strings.TrimSpace(content), &fingerprint
}

func yamlScalar(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			if len(line) > 160 {
				return line[:160]
			}
			return line
		}
	}
	return ""
}
