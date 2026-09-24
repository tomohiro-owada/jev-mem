package jevmem

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"math"
	"sort"
	"time"
)

type reportCell struct {
	Index int
	ID    string
	Name  string
	Value string
	Class string
}

type reportFingerprint struct {
	Cells []reportCell
}

type reportAttribute struct {
	Name       string
	ID         string
	Query      string
	Result     string
	Difference string
	Target     string
	strength   float64
	difference float64
}

type reportResult struct {
	Rank          int
	Label         string
	Anchor        string
	Title         string
	Path          string
	Content       string
	Semantic      string
	Fingerprint   string
	Adjusted      string
	Combined      string
	Coverage      string
	Confidence    string
	SharedCount   int
	Values        *reportFingerprint
	Match         *reportFingerprint
	SharedStrong  []reportAttribute
	Nearest       []reportAttribute
	Differing     []reportAttribute
	HasComparison bool
}

type reportMatrixCell struct {
	Score string
	Class string
	Title string
	Href  string
}

type reportMatrixRow struct {
	Label string
	Cells []reportMatrixCell
}

type reportMatrixItem struct {
	Label string
	Title string
	Href  string
}

type reportView struct {
	Generated string
	Count     int
	Mapped    int
	Query     reportFingerprint
	Results   []reportResult
	Matrix    []reportMatrixRow
	Items     []reportMatrixItem
}

//go:embed report.html
var reportHTML string

var analogReportTemplate = template.Must(template.New("analog-report").Parse(reportHTML))

func RenderAnalogSearchHTML(data AnalogSearchData, generatedAt time.Time) (string, error) {
	if err := ValidateFingerprint(data.QueryFingerprint); err != nil {
		return "", fmt.Errorf("query fingerprint: %w", err)
	}
	view := reportView{
		Generated: generatedAt.Format("2006-01-02 15:04 MST"),
		Count:     len(data.Results),
		Query:     makeReportFingerprint(data.QueryFingerprint),
	}
	for index, result := range data.Results {
		item := reportResult{
			Rank: index + 1, Label: fmt.Sprintf("%02d", index+1),
			Anchor: fmt.Sprintf("result-%d", index+1),
			Title:  result.Title, Path: result.Path, Content: result.Content,
			Semantic:    formatReportScore(result.SemanticScore),
			Fingerprint: formatReportScore(result.FingerprintScore),
			Adjusted:    formatReportScore(result.AdjustedFingerprintScore),
			Combined:    formatReportScore(result.CombinedScore),
		}
		if result.Fingerprint != nil {
			if err := ValidateFingerprint(*result.Fingerprint); err != nil {
				return "", fmt.Errorf("result %d fingerprint: %w", index+1, err)
			}
			values := makeReportFingerprint(*result.Fingerprint)
			match := makeReportMatch(data.QueryFingerprint, *result.Fingerprint)
			item.Values, item.Match = &values, &match
			comparison, err := CompareFingerprints(data.QueryFingerprint, *result.Fingerprint)
			if err == nil {
				item.HasComparison = true
				item.Coverage = formatReportPercent(comparison.Coverage)
				item.Confidence = formatReportPercent(comparison.EffectiveConfidence)
				item.SharedCount = comparison.SharedAttributeCount
				item.SharedStrong, item.Nearest, item.Differing = reportAttributeLists(data.QueryFingerprint, *result.Fingerprint)
			}
			view.Mapped++
		}
		view.Results = append(view.Results, item)
	}
	view.Items, view.Matrix = makeReportMatrix(data)
	var output bytes.Buffer
	if err := analogReportTemplate.Execute(&output, view); err != nil {
		return "", err
	}
	return output.String(), nil
}

func makeReportFingerprint(fingerprint Fingerprint) reportFingerprint {
	result := reportFingerprint{Cells: make([]reportCell, 0, len(FingerprintAttributesV1))}
	for _, definition := range FingerprintAttributesV1 {
		cell := reportCell{Index: definition.Index, ID: definition.ID, Name: definition.NameJA, Value: "不明", Class: "unknown"}
		if value, ok := fingerprint.Values[definition.ID]; ok {
			switch value.Applicability {
			case Applicable:
				if value.Value != nil {
					cell.Value = fmt.Sprintf("%.0f / 100", *value.Value)
					cell.Class = fmt.Sprintf("heat-%d", reportBucket(*value.Value))
				}
			case NotApplicable:
				cell.Value, cell.Class = "適用外", "na"
			}
		}
		result.Cells = append(result.Cells, cell)
	}
	return result
}

func makeReportMatch(query, candidate Fingerprint) reportFingerprint {
	result := reportFingerprint{Cells: make([]reportCell, 0, len(FingerprintAttributesV1))}
	for _, definition := range FingerprintAttributesV1 {
		cell := reportCell{Index: definition.Index, ID: definition.ID, Name: definition.NameJA, Value: "比較不可", Class: "unknown"}
		q, qOK := query.Values[definition.ID]
		c, cOK := candidate.Values[definition.ID]
		if qOK && cOK && q.Applicability == Applicable && c.Applicability == Applicable && q.Value != nil && c.Value != nil {
			difference := math.Abs(*q.Value - *c.Value)
			cell.Value = fmt.Sprintf("現在 %.0f / 過去 %.0f · 差 %.0f", *q.Value, *c.Value, difference)
			cell.Class = fmt.Sprintf("match-%d", reportBucket(100-difference))
		}
		result.Cells = append(result.Cells, cell)
	}
	return result
}

func reportAttributeLists(query, candidate Fingerprint) (strong, nearest, differing []reportAttribute) {
	all := make([]reportAttribute, 0, 100)
	for _, definition := range FingerprintAttributesV1 {
		q, qOK := query.Values[definition.ID]
		c, cOK := candidate.Values[definition.ID]
		if !qOK || !cOK || q.Applicability != Applicable || c.Applicability != Applicable || q.Value == nil || c.Value == nil {
			continue
		}
		diff := math.Abs(*q.Value - *c.Value)
		attr := reportAttribute{
			Name: definition.NameJA, ID: definition.ID, Target: string(definition.Target),
			Query: fmt.Sprintf("%.0f", *q.Value), Result: fmt.Sprintf("%.0f", *c.Value),
			Difference: fmt.Sprintf("%.0f", diff), strength: math.Min(*q.Value, *c.Value), difference: diff,
		}
		all = append(all, attr)
		if attr.strength >= 60 && diff <= 20 {
			strong = append(strong, attr)
		}
	}
	sort.Slice(strong, func(i, j int) bool {
		if strong[i].strength == strong[j].strength {
			return strong[i].difference < strong[j].difference
		}
		return strong[i].strength > strong[j].strength
	})
	sort.Slice(all, func(i, j int) bool { return all[i].difference < all[j].difference })
	nearest = append(nearest, all[:min(5, len(all))]...)
	differing = append(differing, all[max(0, len(all)-4):]...)
	for i, j := 0, len(differing)-1; i < j; i, j = i+1, j-1 {
		differing[i], differing[j] = differing[j], differing[i]
	}
	strong = strong[:min(6, len(strong))]
	return
}

func makeReportMatrix(data AnalogSearchData) ([]reportMatrixItem, []reportMatrixRow) {
	items := []reportMatrixItem{{Label: "Q", Title: "今回の判断", Href: "#query"}}
	fingerprints := []Fingerprint{data.QueryFingerprint}
	for i, result := range data.Results {
		if len(items) >= 13 {
			break
		}
		if result.Fingerprint == nil {
			continue
		}
		items = append(items, reportMatrixItem{
			Label: fmt.Sprintf("%02d", i+1), Title: result.Title, Href: fmt.Sprintf("#result-%d", i+1),
		})
		fingerprints = append(fingerprints, *result.Fingerprint)
	}
	rows := make([]reportMatrixRow, 0, len(items))
	for i, item := range items {
		row := reportMatrixRow{Label: item.Label}
		for j, other := range items {
			cell := reportMatrixCell{Title: item.Title + " ↔ " + other.Title, Href: other.Href}
			if i == j {
				cell.Score, cell.Class = "1.00", "matrix-self"
			} else if comparison, err := CompareFingerprints(fingerprints[i], fingerprints[j]); err == nil {
				cell.Score = fmt.Sprintf("%.2f", comparison.Score)
				cell.Class = fmt.Sprintf("matrix-%d", reportBucket(comparison.Score*100))
				cell.Title += fmt.Sprintf(" · 属性類似度 %.3f · 共通 %d/100 · coverage %.0f%%", comparison.Score, comparison.SharedAttributeCount, comparison.Coverage*100)
			} else {
				cell.Score, cell.Class = "—", "unknown"
				cell.Title += " · 比較不可"
			}
			row.Cells = append(row.Cells, cell)
		}
		rows = append(rows, row)
	}
	return items, rows
}

func reportBucket(value float64) int {
	return max(0, min(9, int(value/10)))
}

func formatReportScore(value float64) string   { return fmt.Sprintf("%.3f", value) }
func formatReportPercent(value float64) string { return fmt.Sprintf("%.0f%%", value*100) }
