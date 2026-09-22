package jevmem

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
)

type DecisionInput struct {
	Statement         string   `json:"decision"`
	Rationale         string   `json:"rationale"`
	Evidence          []string `json:"evidence,omitempty"`
	Context           string   `json:"context,omitempty"`
	DecisionTime      string   `json:"decision_time,omitempty"`
	FocalOption       string   `json:"focal_option"`
	ReferenceOption   string   `json:"reference_option,omitempty"`
	ChosenResponse    string   `json:"chosen_response"`
	EvaluationHorizon string   `json:"evaluation_horizon,omitempty"`
}

func (d DecisionInput) SemanticText() string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "Decision:\n%s\n\nRationale:\n%s", strings.TrimSpace(d.Statement), strings.TrimSpace(d.Rationale))
	if len(d.Evidence) > 0 {
		fmt.Fprintln(&b, "\n\nEvidence:")
		for _, evidence := range d.Evidence {
			if evidence = strings.TrimSpace(evidence); evidence != "" {
				fmt.Fprintf(&b, "- %s\n", evidence)
			}
		}
	}
	if context := strings.TrimSpace(d.Context); context != "" {
		fmt.Fprintf(&b, "\nContext:\n%s", context)
	}
	fmt.Fprintf(&b, "\n\nDecision frame:\n- Focal option: %s\n- Reference option: %s\n- Chosen response: %s\n- Evaluation horizon: %s",
		strings.TrimSpace(d.FocalOption), strings.TrimSpace(d.ReferenceOption), strings.TrimSpace(d.ChosenResponse), strings.TrimSpace(d.EvaluationHorizon))
	return strings.TrimSpace(b.String())
}

func (d DecisionInput) Validate() error {
	if d.Statement == "" {
		return errors.New("decision is required")
	}
	if d.Rationale == "" {
		return errors.New("rationale is required")
	}
	if d.FocalOption == "" {
		return errors.New("focal_option is required")
	}
	if d.ChosenResponse == "" {
		return errors.New("chosen_response is required")
	}
	return nil
}

func (d DecisionInput) JevState() map[string]any {
	return map[string]any{
		"decision":           d.Statement,
		"rationale":          d.Rationale,
		"evidence":           d.Evidence,
		"context":            d.Context,
		"decision_time":      d.DecisionTime,
		"focal_option":       d.FocalOption,
		"reference_option":   d.ReferenceOption,
		"chosen_response":    d.ChosenResponse,
		"evaluation_horizon": d.EvaluationHorizon,
	}
}

type JevEvaluator interface {
	Evaluate(ctx context.Context, state any, questions map[string]JevQuestion) (JevResponse, error)
}

type FingerprintExtractor interface {
	Extract(ctx context.Context, decision DecisionInput) (Fingerprint, error)
}

type JevFingerprintExtractor struct {
	Evaluator        JevEvaluator
	ExtractorVersion string
	BatchSize        int
}

func (e *JevFingerprintExtractor) Extract(ctx context.Context, decision DecisionInput) (Fingerprint, error) {
	if err := decision.Validate(); err != nil {
		return Fingerprint{}, err
	}
	if e == nil || e.Evaluator == nil {
		return Fingerprint{}, errors.New("Jev evaluator is required")
	}
	batchSize := e.BatchSize
	if batchSize <= 0 {
		batchSize = 25
	}
	fingerprint := Fingerprint{
		SchemaVersion:    FingerprintSchemaV1,
		Extractor:        fingerprintExtractorName(e.Evaluator),
		ExtractorVersion: e.ExtractorVersion,
		Values:           make(map[string]AttributeValue, len(FingerprintAttributesV1)),
	}
	state := decision.JevState()
	for start := 0; start < len(FingerprintAttributesV1); start += batchSize {
		end := start + batchSize
		if end > len(FingerprintAttributesV1) {
			end = len(FingerprintAttributesV1)
		}
		definitions := FingerprintAttributesV1[start:end]
		questions := make(map[string]JevQuestion, len(definitions)*2)
		for _, definition := range definitions {
			questions[definition.ID+".applicability"] = applicabilityQuestion(definition)
			questions[definition.ID+".score"] = scoreQuestion(definition)
		}
		response, err := e.Evaluator.Evaluate(ctx, state, questions)
		if err != nil {
			return Fingerprint{}, fmt.Errorf("extract fingerprint attributes %d-%d: %w", start+1, end, err)
		}
		if fingerprint.ExtractorVersion == "" {
			fingerprint.ExtractorVersion = response.Model
		}
		for _, definition := range definitions {
			value, err := attributeValueFromJev(definition, response.Answers)
			if err != nil {
				return Fingerprint{}, err
			}
			fingerprint.Values[definition.ID] = value
		}
	}
	if err := ValidateFingerprint(fingerprint); err != nil {
		return Fingerprint{}, err
	}
	return fingerprint, nil
}

func fingerprintExtractorName(evaluator JevEvaluator) string {
	if _, ok := evaluator.(*LocalLayaEvaluator); ok {
		return "laya-mlx"
	}
	return "jev"
}

func applicabilityQuestion(definition AttributeDefinition) JevQuestion {
	highAnchor := attributeHighAnchorsV1[definition.ID]
	return JevQuestion{
		Type: "choice",
		Instructions: map[string]any{
			"attribute_id": definition.ID,
			"attribute":    definition.NameJA,
			"target":       definition.Target,
			"high_anchor":  highAnchor,
			"question":     "Decisionの記録だけを根拠に、この属性を評価できるか判定してください。記述が足りない場合はunknown、定義上対象外ならnot_applicableを選んでください。推測でapplicableにしないでください。",
		},
		Criteria: map[string]string{
			"applicable":     "記録に評価根拠があり、属性値を採点できる。",
			"unknown":        "属性は適用可能だが、記録の情報が不足して採点できない。",
			"not_applicable": "このDecisionには定義上この属性を適用できない。",
		},
	}
}

func scoreQuestion(definition AttributeDefinition) JevQuestion {
	highAnchor := attributeHighAnchorsV1[definition.ID]
	return JevQuestion{
		Type: "score",
		Instructions: map[string]any{
			"attribute_id": definition.ID,
			"attribute":    definition.NameJA,
			"target":       definition.Target,
			"zero_anchor":  "この属性の高得点側の性質が存在しない、または反対側の性質が明確である",
			"high_anchor":  highAnchor,
			"question":     scoreInstruction(definition.Target),
		},
		Criteria: []string{
			"0点: " + definition.NameJA + "について、高得点側の性質が存在しない、または反対側の性質が明確である。",
			"25点: " + highAnchor + "という性質が弱く存在する。",
			"50点: " + highAnchor + "という性質が中程度である。",
			"75点: " + highAnchor + "という性質が強い。",
			"100点: " + highAnchor + "。",
		},
	}
}

func scoreInstruction(target AttributeTarget) string {
	switch target {
	case TargetContext:
		return "判断時点の状況、およびfocal_optionを実行した場合の性質として、この属性の強さを評価してください。chosen_responseの性質と混同しないでください。"
	case TargetCriterion:
		return "この属性に対応する価値・条件が、Rationaleで実際にどれほど判断を左右したか評価してください。状況にその性質が存在するだけでは高得点にしないでください。"
	case TargetMethod:
		return "この判断方法がRationale内で実際にどれほど使われ、結論を左右したか評価してください。"
	case TargetStrategy:
		return "focal_optionの性質ではなく、chosen_responseとして実際に選んだ対応にこの性質がどれほど含まれるか評価してください。"
	default:
		return "この属性の強さを評価してください。"
	}
}

func attributeValueFromJev(definition AttributeDefinition, answers map[string]JevAnswer) (AttributeValue, error) {
	appAnswer, ok := answers[definition.ID+".applicability"]
	if !ok || appAnswer.Type != "choice" || appAnswer.Choice == "" {
		return AttributeValue{}, fmt.Errorf("missing applicability answer for %s", definition.ID)
	}
	appConfidence := answerConfidence(appAnswer)
	switch appAnswer.Choice {
	case string(Unknown):
		return AttributeValue{Applicability: Unknown, Observation: Inferred, Confidence: appConfidence}, nil
	case string(NotApplicable):
		return AttributeValue{Applicability: NotApplicable, Observation: Inferred, Confidence: appConfidence}, nil
	case string(Applicable):
	default:
		return AttributeValue{}, fmt.Errorf("invalid applicability answer %q for %s", appAnswer.Choice, definition.ID)
	}
	scoreAnswer, ok := answers[definition.ID+".score"]
	if !ok || scoreAnswer.Type != "score" || scoreAnswer.Score == nil {
		return AttributeValue{}, fmt.Errorf("missing score answer for %s", definition.ID)
	}
	if *scoreAnswer.Score < 0 || *scoreAnswer.Score > 4 {
		return AttributeValue{}, fmt.Errorf("score for %s is outside the five-level rubric", definition.ID)
	}
	value := *scoreAnswer.Score * 25
	confidence := math.Min(appConfidence, answerConfidence(scoreAnswer))
	return AttributeValue{
		Value:         &value,
		Applicability: Applicable,
		Observation:   Inferred,
		Confidence:    confidence,
		EvidenceRefs:  defaultEvidenceRefs(definition.Target),
	}, nil
}

func answerConfidence(answer JevAnswer) float64 {
	if answer.Confidence == nil {
		return 0
	}
	return math.Max(0, math.Min(1, *answer.Confidence))
}

func defaultEvidenceRefs(target AttributeTarget) []string {
	switch target {
	case TargetContext:
		return []string{"context", "evidence", "focal_option", "reference_option"}
	case TargetCriterion, TargetMethod:
		return []string{"rationale", "evidence"}
	case TargetStrategy:
		return []string{"chosen_response", "decision"}
	default:
		return nil
	}
}
