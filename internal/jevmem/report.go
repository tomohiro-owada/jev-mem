package jevmem

import (
	"bytes"
	"fmt"
	"html/template"
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

type reportResult struct {
	Rank                     int
	Title                    string
	Path                     string
	Content                  string
	SemanticScore            string
	FingerprintScore         string
	AdjustedFingerprintScore string
	CombinedScore            string
	Fingerprint              *reportFingerprint
	Matching                 []AttributeComparison
	Differing                []AttributeComparison
}

type reportView struct {
	Generated string
	Query     reportFingerprint
	Results   []reportResult
}

func RenderAnalogSearchHTML(data AnalogSearchData, generatedAt time.Time) (string, error) {
	if err := ValidateFingerprint(data.QueryFingerprint); err != nil {
		return "", fmt.Errorf("query fingerprint: %w", err)
	}
	view := reportView{Generated: generatedAt.Format(time.RFC3339), Query: makeReportFingerprint(data.QueryFingerprint)}
	for index, result := range data.Results {
		item := reportResult{
			Rank:                     index + 1,
			Title:                    result.Title,
			Path:                     result.Path,
			Content:                  result.Content,
			SemanticScore:            formatReportScore(result.SemanticScore),
			FingerprintScore:         formatReportScore(result.FingerprintScore),
			AdjustedFingerprintScore: formatReportScore(result.AdjustedFingerprintScore),
			CombinedScore:            formatReportScore(result.CombinedScore),
		}
		if result.Fingerprint != nil {
			fingerprint := makeReportFingerprint(*result.Fingerprint)
			item.Fingerprint = &fingerprint
		}
		if result.FingerprintSimilarity != nil {
			item.Matching = result.FingerprintSimilarity.TopMatching
			item.Differing = result.FingerprintSimilarity.TopDiffering
		}
		view.Results = append(view.Results, item)
	}
	var output bytes.Buffer
	if err := analogReportTemplate.Execute(&output, view); err != nil {
		return "", err
	}
	return output.String(), nil
}

func makeReportFingerprint(fingerprint Fingerprint) reportFingerprint {
	result := reportFingerprint{Cells: make([]reportCell, 0, len(FingerprintAttributesV1))}
	for _, definition := range FingerprintAttributesV1 {
		cell := reportCell{Index: definition.Index, ID: definition.ID, Name: definition.NameJA, Value: "unknown", Class: "unknown"}
		if value, ok := fingerprint.Values[definition.ID]; ok {
			switch value.Applicability {
			case Applicable:
				cell.Value = fmt.Sprintf("%.1f", *value.Value)
				bucket := int(*value.Value / 10)
				if bucket > 9 {
					bucket = 9
				}
				cell.Class = fmt.Sprintf("heat-%d", bucket)
			case NotApplicable:
				cell.Value, cell.Class = "not applicable", "na"
			}
		}
		result.Cells = append(result.Cells, cell)
	}
	return result
}

func formatReportScore(value float64) string { return fmt.Sprintf("%.3f", value) }

var analogReportTemplate = template.Must(template.New("analog-report").Parse(`<!doctype html>
<html lang="ja">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>jev-mem analogical search report</title>
<style>
:root{color-scheme:dark;--bg:#07111f;--panel:#0f1c2e;--line:#26364c;--text:#e6edf7;--muted:#91a2ba;--accent:#45c8e8}*{box-sizing:border-box}body{margin:0;background:radial-gradient(circle at top,#132642 0,#07111f 42rem);color:var(--text);font:14px/1.55 Inter,ui-sans-serif,system-ui,-apple-system,sans-serif}.page{max-width:1440px;margin:auto;padding:40px 28px 80px}h1{font-size:30px;margin:0}.lead{color:var(--muted);margin:7px 0 30px}.query,.card{background:color-mix(in srgb,var(--panel) 94%,transparent);border:1px solid var(--line);border-radius:18px;box-shadow:0 16px 40px #0005}.query{padding:22px;margin-bottom:28px}.query-layout,.result-layout{display:grid;grid-template-columns:minmax(300px,480px) 1fr;gap:26px;align-items:start}.heatmap{display:grid;grid-template-columns:repeat(10,minmax(22px,1fr));gap:5px;aspect-ratio:1;max-width:480px}.cell{display:grid;place-items:center;border-radius:5px;color:#f8fafc;font-size:10px;font-weight:750;border:1px solid #ffffff12;cursor:help}.heat-0{background:#b8e8f3;color:#10202b}.heat-1{background:#95dcea;color:#10202b}.heat-2{background:#72cfe2}.heat-3{background:#4fc1da}.heat-4{background:#29acc9}.heat-5{background:#148da9}.heat-6{background:#08718b}.heat-7{background:#07586e}.heat-8{background:#084256}.heat-9{background:#082f40}.unknown{background:#334155}.na{background:#111827;color:#64748b}.results{display:grid;gap:18px}.card{padding:22px}.rank{display:inline-grid;place-items:center;width:34px;height:34px;border-radius:9px;background:#163a51;color:#7ee4fa;font-weight:800}.card h2{display:inline;margin:0 0 0 10px;font-size:19px}.path{color:var(--muted);font:12px ui-monospace,SFMono-Regular,Menlo,monospace;margin:6px 0 16px}.scores{display:grid;grid-template-columns:repeat(4,minmax(105px,1fr));gap:8px;margin-bottom:18px}.score{padding:10px 12px;border:1px solid var(--line);border-radius:10px;background:#0a1626}.score b{display:block;font-size:18px;color:#8ae9fb}.score span{color:var(--muted);font-size:11px}.attributes{display:grid;grid-template-columns:1fr 1fr;gap:14px}.attributes h3{margin:0 0 6px;font-size:13px}.attributes ul{padding-left:18px;margin:0;color:var(--muted)}details{margin-top:16px;border-top:1px solid var(--line);padding-top:12px}summary{cursor:pointer;color:#b8c8db}.content{white-space:pre-wrap;color:#b8c8db;max-height:420px;overflow:auto}.legend{display:flex;gap:12px;flex-wrap:wrap;color:var(--muted);font-size:12px;margin-top:12px}.swatch{width:12px;height:12px;display:inline-block;border-radius:3px;margin-right:4px}@media(max-width:800px){.query-layout,.result-layout{grid-template-columns:1fr}.scores{grid-template-columns:1fr 1fr}.page{padding:24px 14px}.cell{font-size:8px}}
</style>
</head>
<body><main class="page">
<h1>Decision Fingerprint Search</h1><p class="lead">類似順の10×10属性マップ · generated {{.Generated}}</p>
<section class="query"><div class="query-layout"><div><h2>Query fingerprint</h2><p class="lead">各セルは固定された同一属性です。画像ではなく元の属性値で検索しています。</p><div class="legend"><span><i class="swatch heat-0"></i>low</span><span><i class="swatch heat-9"></i>high</span><span><i class="swatch unknown"></i>unknown</span></div></div>{{template "heatmap" .Query}}</div></section>
<section class="results">{{range .Results}}<article class="card"><div><span class="rank">{{.Rank}}</span><h2>{{.Title}}</h2></div><div class="path">{{.Path}}</div><div class="scores"><div class="score"><b>{{.CombinedScore}}</b><span>combined</span></div><div class="score"><b>{{.FingerprintScore}}</b><span>fingerprint</span></div><div class="score"><b>{{.AdjustedFingerprintScore}}</b><span>adjusted fingerprint</span></div><div class="score"><b>{{.SemanticScore}}</b><span>semantic</span></div></div><div class="result-layout">{{if .Fingerprint}}{{template "heatmap" .Fingerprint}}{{else}}<p>Fingerprint unavailable</p>{{end}}<div><div class="attributes"><div><h3>近い属性</h3><ul>{{range .Matching}}<li>{{.AttributeID}} ({{printf "%.0f" .Left}} ↔ {{printf "%.0f" .Right}})</li>{{end}}</ul></div><div><h3>異なる属性</h3><ul>{{range .Differing}}<li>{{.AttributeID}} ({{printf "%.0f" .Left}} ↔ {{printf "%.0f" .Right}})</li>{{end}}</ul></div></div><details><summary>Decision本文</summary><div class="content">{{.Content}}</div></details></div></div></article>{{else}}<p>検索結果はありません。</p>{{end}}</section>
</main></body></html>
{{define "heatmap"}}<div class="heatmap" role="img" aria-label="10 by 10 decision fingerprint heatmap">{{range .Cells}}<div class="cell {{.Class}}" title="{{printf "%02d" .Index}} {{.Name}} ({{.ID}}): {{.Value}}">{{printf "%02d" .Index}}</div>{{end}}</div>{{end}}`))
