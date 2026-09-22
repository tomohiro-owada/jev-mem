package jevmem

import (
	"errors"
	"fmt"
	"math"
	"sort"
)

const FingerprintSchemaV1 = 1

type AttributeTarget string

const (
	TargetContext   AttributeTarget = "context"
	TargetCriterion AttributeTarget = "criterion"
	TargetMethod    AttributeTarget = "method"
	TargetStrategy  AttributeTarget = "strategy"
)

type Applicability string

const (
	Applicable    Applicability = "applicable"
	Unknown       Applicability = "unknown"
	NotApplicable Applicability = "not_applicable"
)

type Observation string

const (
	Observed Observation = "observed"
	Inferred Observation = "inferred"
)

type AttributeDefinition struct {
	Index  int             `json:"index"`
	ID     string          `json:"id"`
	NameJA string          `json:"name_ja"`
	Target AttributeTarget `json:"target"`
}

type AttributeValue struct {
	Value         *float64      `json:"value,omitempty"`
	Applicability Applicability `json:"applicability"`
	Observation   Observation   `json:"observation,omitempty"`
	Confidence    float64       `json:"confidence,omitempty"`
	EvidenceRefs  []string      `json:"evidence_refs,omitempty"`
}

type Fingerprint struct {
	SchemaVersion    int                       `json:"schema_version"`
	Extractor        string                    `json:"extractor,omitempty"`
	ExtractorVersion string                    `json:"extractor_version,omitempty"`
	Values           map[string]AttributeValue `json:"values"`
}

type AttributeComparison struct {
	AttributeID string  `json:"attribute_id"`
	Target      string  `json:"target"`
	Left        float64 `json:"left"`
	Right       float64 `json:"right"`
	Difference  float64 `json:"difference"`
	Similarity  float64 `json:"similarity"`
}

type FingerprintSimilarity struct {
	Score                float64               `json:"score"`
	Layers               map[string]float64    `json:"layers"`
	Coverage             float64               `json:"coverage"`
	SharedAttributeCount int                   `json:"shared_attribute_count"`
	EffectiveConfidence  float64               `json:"effective_confidence"`
	TopMatching          []AttributeComparison `json:"top_matching_attributes"`
	TopDiffering         []AttributeComparison `json:"top_differing_attributes"`
	LayerCoverage        map[string]float64    `json:"layer_coverage"`
	Warnings             []string              `json:"warnings"`
}

var layerWeights = map[AttributeTarget]float64{
	TargetContext:   0.30,
	TargetCriterion: 0.45,
	TargetMethod:    0.15,
	TargetStrategy:  0.10,
}

// FingerprintAttributesV1 is ordered and stable. Reordering or redefining an
// item requires a new schema version, even though search addresses values by ID.
var FingerprintAttributesV1 = []AttributeDefinition{
	{1, "state_uncertainty", "現状の未把握度", TargetContext},
	{2, "outcome_variability", "結果の変動性", TargetContext},
	{3, "causal_model_uncertainty", "因果機構の不確かさ", TargetContext},
	{4, "evidence_credibility_risk", "証拠の信用上の懸念", TargetContext},
	{5, "evidence_indirectness", "証拠の間接性", TargetContext},
	{6, "evidence_conflict", "証拠の不一致", TargetContext},
	{7, "evidence_context_mismatch", "証拠と適用環境のずれ", TargetContext},
	{8, "evidence_staleness", "証拠の陳腐化", TargetContext},
	{9, "information_asymmetry", "情報の非対称性", TargetContext},
	{10, "precedent_scarcity", "比較可能な前例の不足", TargetContext},
	{11, "option_ranking_sensitivity", "案の順位の不安定さ", TargetContext},
	{12, "predecision_learning_potential", "決定前の学習余地", TargetContext},
	{13, "decision_deadline_pressure", "決定期限の圧力", TargetContext},
	{14, "delay_cost", "延期の損失", TargetContext},
	{15, "opportunity_window_transience", "好機の短命さ", TargetContext},
	{16, "environment_change_rate", "環境の変化速度", TargetContext},
	{17, "benefit_realization_delay", "便益発現までの長さ", TargetContext},
	{18, "impact_duration", "影響の持続期間", TargetContext},
	{19, "feedback_delay", "結果判明の遅さ", TargetContext},
	{20, "success_measurability", "成功の測定可能性", TargetContext},
	{21, "downside_severity", "通常の不首尾による損失規模", TargetContext},
	{22, "downside_likelihood", "許容下限を割る見込み", TargetContext},
	{23, "upside_potential", "上振れ便益の大きさ", TargetContext},
	{24, "tail_risk_exposure", "極端損害への曝露", TargetContext},
	{25, "direct_impact_scope", "直接影響の広さ", TargetContext},
	{26, "spillover_potential", "間接的な波及性", TargetContext},
	{27, "risk_coupling", "既存リスクとの連動", TargetContext},
	{28, "benefit_burden_separation", "受益者と負担者の分離", TargetContext},
	{29, "binding_commitment_duration", "拘束期間", TargetContext},
	{30, "reversal_cost", "撤回の資源負担", TargetContext},
	{31, "reversal_latency", "撤回に必要な時間", TargetContext},
	{32, "residual_irreversibility", "撤回後に残る不可逆性", TargetContext},
	{33, "exit_permission_dependence", "離脱の他者同意依存", TargetContext},
	{34, "future_option_foreclosure", "将来の選択肢の閉鎖", TargetContext},
	{35, "viable_alternative_scarcity", "実行可能な代替案の不足", TargetContext},
	{36, "choice_exclusivity", "選択の排他性", TargetContext},
	{37, "trial_feasibility", "小さく試せる度合い", TargetContext},
	{38, "commitment_divisibility", "コミットメントの分割可能性", TargetContext},
	{39, "upfront_resource_burden", "初期資源負担", TargetContext},
	{40, "recurring_resource_burden", "継続資源負担", TargetContext},
	{41, "resource_slack_scarcity", "資源余力の不足", TargetContext},
	{42, "capability_gap", "必要能力との隔たり", TargetContext},
	{43, "implementation_complexity", "実行構造の複雑さ", TargetContext},
	{44, "coordination_burden", "関係者間の調整負担", TargetContext},
	{45, "external_dependency", "外部依存", TargetContext},
	{46, "outcome_controllability", "結果の制御可能性", TargetContext},
	{47, "failure_detectability", "失敗兆候の検知可能性", TargetContext},
	{48, "recovery_difficulty", "失敗後の復旧困難度", TargetContext},
	{49, "objective_ambiguity", "目標の未確定性", TargetContext},
	{50, "stakeholder_conflict", "利害の対立", TargetContext},
	{51, "authority_fragmentation", "決定権限の分散", TargetContext},
	{52, "incentive_misalignment", "誘因のずれ", TargetContext},
	{53, "formal_constraint_intensity", "明文化された制約の強さ", TargetContext},
	{54, "impact_inequality", "影響配分の偏り", TargetContext},
	{55, "knowledge_concentration", "重要知識の集中", TargetContext},
	{56, "decision_implementation_separation", "決定者と実行者の分離", TargetContext},
	{57, "loss_avoidance_priority", "損失回避の重視", TargetCriterion},
	{58, "catastrophe_avoidance_priority", "破局回避の重視", TargetCriterion},
	{59, "upside_priority", "上振れ追求の重視", TargetCriterion},
	{60, "efficiency_priority", "効率の重視", TargetCriterion},
	{61, "liquidity_priority", "当面の支払余力の重視", TargetCriterion},
	{62, "long_term_value_priority", "長期価値の重視", TargetCriterion},
	{63, "speed_priority", "速さの重視", TargetCriterion},
	{64, "performance_priority", "到達性能の重視", TargetCriterion},
	{65, "reliability_priority", "安定達成の重視", TargetCriterion},
	{66, "robustness_priority", "条件変化への頑健さの重視", TargetCriterion},
	{67, "adaptability_priority", "変更容易性の重視", TargetCriterion},
	{68, "optionality_priority", "将来の選択肢の重視", TargetCriterion},
	{69, "autonomy_priority", "自律的な制御の重視", TargetCriterion},
	{70, "simplicity_priority", "単純さの重視", TargetCriterion},
	{71, "interoperability_priority", "接続・互換性の重視", TargetCriterion},
	{72, "scalability_priority", "拡大への対応の重視", TargetCriterion},
	{73, "learning_priority", "学習の重視", TargetCriterion},
	{74, "evidence_assurance_priority", "根拠の確かさの重視", TargetCriterion},
	{75, "continuity_priority", "継続性の重視", TargetCriterion},
	{76, "reversibility_priority", "やり直し可能性の重視", TargetCriterion},
	{77, "fair_distribution_priority", "配分の公平性の重視", TargetCriterion},
	{78, "stakeholder_acceptance_priority", "関係者の受容の重視", TargetCriterion},
	{79, "procedural_legitimacy_priority", "手続き上の正当性の重視", TargetCriterion},
	{80, "compliance_priority", "明文化された義務の遵守の重視", TargetCriterion},
	{81, "affected_party_agency_priority", "影響を受ける人の主体性の重視", TargetCriterion},
	{82, "trust_reputation_priority", "信頼・評判維持の重視", TargetCriterion},
	{83, "confidentiality_priority", "情報保護の重視", TargetCriterion},
	{84, "explainability_priority", "説明・追跡可能性の重視", TargetCriterion},
	{85, "strategic_alignment_priority", "上位方針との整合の重視", TargetCriterion},
	{86, "capability_accumulation_priority", "再利用できる能力の蓄積の重視", TargetCriterion},
	{87, "expected_value_use", "期待値による判断", TargetMethod},
	{88, "worst_case_use", "最悪条件による判断", TargetMethod},
	{89, "hard_threshold_use", "足切り条件による判断", TargetMethod},
	{90, "satisficing_use", "十分条件で探索を止める判断", TargetMethod},
	{91, "causal_model_use", "因果説明による判断", TargetMethod},
	{92, "precedent_use", "前例による判断", TargetMethod},
	{93, "expert_judgment_use", "専門家判断への依拠", TargetMethod},
	{94, "scenario_analysis_use", "複数シナリオによる判断", TargetMethod},
	{95, "compensatory_tradeoff_use", "基準間の交換による判断", TargetMethod},
	{96, "immediate_commitment_extent", "即時に確定した範囲", TargetStrategy},
	{97, "precommitment_learning_extent", "本格決定前の情報取得", TargetStrategy},
	{98, "staged_commitment_extent", "段階的な確定", TargetStrategy},
	{99, "risk_mitigation_extent", "予防・緩和策の組込み", TargetStrategy},
	{100, "retained_option_extent", "選択後に残した選択肢", TargetStrategy},
}

func AttributeDefinitions(schemaVersion int) ([]AttributeDefinition, error) {
	if schemaVersion != FingerprintSchemaV1 {
		return nil, fmt.Errorf("unsupported fingerprint schema version: %d", schemaVersion)
	}
	out := make([]AttributeDefinition, len(FingerprintAttributesV1))
	copy(out, FingerprintAttributesV1)
	return out, nil
}

func ValidateFingerprint(fp Fingerprint) error {
	defs, err := AttributeDefinitions(fp.SchemaVersion)
	if err != nil {
		return err
	}
	known := make(map[string]struct{}, len(defs))
	for _, def := range defs {
		known[def.ID] = struct{}{}
	}
	for id, value := range fp.Values {
		if _, ok := known[id]; !ok {
			return fmt.Errorf("unknown fingerprint attribute: %s", id)
		}
		switch value.Applicability {
		case Applicable:
			if value.Value == nil {
				return fmt.Errorf("attribute %s is applicable but has no value", id)
			}
			if *value.Value < 0 || *value.Value > 100 {
				return fmt.Errorf("attribute %s value must be between 0 and 100", id)
			}
		case Unknown, NotApplicable:
			if value.Value != nil {
				return fmt.Errorf("attribute %s must not have a value when applicability is %s", id, value.Applicability)
			}
		default:
			return fmt.Errorf("attribute %s has invalid applicability %q", id, value.Applicability)
		}
		if value.Confidence < 0 || value.Confidence > 1 {
			return fmt.Errorf("attribute %s confidence must be between 0 and 1", id)
		}
	}
	return nil
}

func CompareFingerprints(left, right Fingerprint) (FingerprintSimilarity, error) {
	if left.SchemaVersion != right.SchemaVersion {
		return FingerprintSimilarity{}, errors.New("fingerprint schema versions do not match")
	}
	if err := ValidateFingerprint(left); err != nil {
		return FingerprintSimilarity{}, fmt.Errorf("left fingerprint: %w", err)
	}
	if err := ValidateFingerprint(right); err != nil {
		return FingerprintSimilarity{}, fmt.Errorf("right fingerprint: %w", err)
	}
	defs, _ := AttributeDefinitions(left.SchemaVersion)
	type accumulator struct {
		weightedSquaredDistance float64
		weight                  float64
		confidence              float64
		shared                  int
		total                   int
	}
	acc := map[AttributeTarget]*accumulator{}
	for target := range layerWeights {
		acc[target] = &accumulator{}
	}
	comparisons := make([]AttributeComparison, 0, len(defs))
	for _, def := range defs {
		a := acc[def.Target]
		a.total++
		lv, lok := left.Values[def.ID]
		rv, rok := right.Values[def.ID]
		leftKnown := lok && lv.Applicability == Applicable && lv.Value != nil
		rightKnown := rok && rv.Applicability == Applicable && rv.Value != nil
		if !leftKnown || !rightKnown {
			continue
		}
		weight := math.Sqrt(lv.Confidence * rv.Confidence)
		if weight == 0 {
			continue
		}
		difference := math.Abs(*lv.Value - *rv.Value)
		normalized := difference / 100
		a.weightedSquaredDistance += weight * normalized * normalized
		a.weight += weight
		a.confidence += weight
		a.shared++
		comparisons = append(comparisons, AttributeComparison{
			AttributeID: def.ID,
			Target:      string(def.Target),
			Left:        *lv.Value,
			Right:       *rv.Value,
			Difference:  difference,
			Similarity:  1 - normalized,
		})
	}

	result := FingerprintSimilarity{
		Layers:        map[string]float64{},
		LayerCoverage: map[string]float64{},
		Warnings:      []string{},
	}
	var weightedScore, usedLayerWeight, confidenceSum float64
	for target, configuredWeight := range layerWeights {
		a := acc[target]
		if a.total > 0 {
			result.LayerCoverage[string(target)] = float64(a.shared) / float64(a.total)
		}
		result.SharedAttributeCount += a.shared
		confidenceSum += a.confidence
		if a.weight == 0 {
			result.Warnings = append(result.Warnings, fmt.Sprintf("no comparable %s attributes", target))
			continue
		}
		score := 1 - math.Sqrt(a.weightedSquaredDistance/a.weight)
		result.Layers[string(target)] = score
		weightedScore += configuredWeight * score
		usedLayerWeight += configuredWeight
	}
	if result.SharedAttributeCount == 0 || usedLayerWeight == 0 {
		return result, errors.New("fingerprints have no comparable attributes with positive confidence")
	}
	result.Score = weightedScore / usedLayerWeight
	result.Coverage = float64(result.SharedAttributeCount) / float64(len(defs))
	result.EffectiveConfidence = confidenceSum / float64(result.SharedAttributeCount)

	sort.Slice(comparisons, func(i, j int) bool {
		if comparisons[i].Difference == comparisons[j].Difference {
			return comparisons[i].AttributeID < comparisons[j].AttributeID
		}
		return comparisons[i].Difference < comparisons[j].Difference
	})
	result.TopMatching = takeComparisons(comparisons, 5)
	sort.Slice(comparisons, func(i, j int) bool {
		if comparisons[i].Difference == comparisons[j].Difference {
			return comparisons[i].AttributeID < comparisons[j].AttributeID
		}
		return comparisons[i].Difference > comparisons[j].Difference
	})
	result.TopDiffering = takeComparisons(comparisons, 5)
	return result, nil
}

func takeComparisons(values []AttributeComparison, limit int) []AttributeComparison {
	if len(values) < limit {
		limit = len(values)
	}
	out := make([]AttributeComparison, limit)
	copy(out, values[:limit])
	return out
}
