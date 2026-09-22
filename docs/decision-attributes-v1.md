# Decision Fingerprint v1: 意思決定構造検索の100属性

## 目的

本設計は、Decision / Rationale / Evidence / Context から意思決定の構造を100個の固定属性として評価し、既存のSemantic Embeddingとは異なる近傍を検索するためのものである。

- Semantic検索: 「何について判断したか」の近さ
- Fingerprint検索: 「どのような状況で、何を重視し、どう判断し、どう対応したか」の近さ
- 10×10ヒートマップ: 同じ100属性を人間が認識するための表示。画像検索には使用しない

この100属性は運用開始時の仮説であり、統計的独立性や検索精度を保証するものではない。実データで再現性・相関・検索寄与を検証し、schema versionを上げて改訂する。

## 1. 評価単位を先に固定する

属性抽出前に、必ず次を確定する。

| フィールド | 意味 |
|---|---|
| `decision_time` | 評価の基準時点 |
| `focal_option` | 主たる検討対象案 |
| `reference_option` | 比較対象案または現状維持 |
| `chosen_response` | 実際に選んだ対応 |
| `evaluation_horizon` | 便益・損失を評価する期間 |

複数の対象案が混在する場合は、原則としてDecisionを分割する。モデルが暗黙に「最も危険な案」などをfocal optionとして選んではならない。

## 2. 属性の4層

| target | 件数 | 評価対象 |
|---|---:|---|
| `context` | 56 | 判断時点の状況と、focal optionを実行した場合の性質 |
| `criterion` | 30 | Rationaleで実際に重視した価値・条件 |
| `method` | 9 | 比較・推論に実際に使った判断方法 |
| `strategy` | 5 | chosen responseの性質 |

対象案の性質と、判断での重みと、実際に選んだ対応を分離する。例えば年間契約を見送った場合、契約の拘束期間は `context`、将来の選択肢を重視した度合いは `criterion`、今回確定した範囲は `strategy` である。

## 3. スコア共通仕様

各属性は次の構造を持つ。

```json
{
  "attribute_id": "state_uncertainty",
  "value": 75,
  "applicability": "applicable",
  "observation": "inferred",
  "confidence": 0.78,
  "evidence_refs": ["context:2", "evidence:1"],
  "extractor_version": "jev/..."
}
```

- `value`: 0〜100。初期運用では0 / 25 / 50 / 75 / 100を基本アンカーとし、見せかけの精密さを避ける
- `applicability`: `applicable / not_applicable / unknown`
- `unknown`: 適用可能だが情報が足りない
- `not_applicable`: 定義上そのDecisionには適用できない
- `observation`: `observed / inferred`
- 記述がないことを0や50として扱わない
- `criterion`は言及回数ではなく、比較、棄却、譲歩、決め手への寄与で採点する
- 個人の恒常的な性格は推測せず、「この判断で何を重視したか」だけを採点する

## 4. 最終100属性

以下の「100」は高得点側の意味を示す。0は原則としてその反対側、50は中間状態とする。実装スキーマでは各属性に個別の0 / 50 / 100アンカーを保持する。

### A. 不確実性と証拠（context: 1–12）

| # | attribute_id | 日本語名 | 100の意味 |
|---:|---|---|---|
| 1 | `state_uncertainty` | 現状の未把握度 | 判断に重要な現在の状態・事実がほぼ不明 |
| 2 | `outcome_variability` | 結果の変動性 | 同じ条件でも結果が大きく振れる |
| 3 | `causal_model_uncertainty` | 因果機構の不確かさ | 行動から結果へ至る仕組みがほぼ不明 |
| 4 | `evidence_credibility_risk` | 証拠の信用上の懸念 | 主要材料の正確性・誠実性・測定品質に重大な疑い |
| 5 | `evidence_indirectness` | 証拠の間接性 | ほぼ代理指標・間接観測だけで評価している |
| 6 | `evidence_conflict` | 証拠の不一致 | 重要な証拠が互いに強く対立する |
| 7 | `evidence_context_mismatch` | 証拠と適用環境のずれ | 証拠の環境と今回の主要条件が大きく異なる |
| 8 | `evidence_staleness` | 証拠の陳腐化 | 主要前提の変化により証拠が現在を表さない |
| 9 | `information_asymmetry` | 情報の非対称性 | 相手方・一部主体が決定的に多くの重要情報を持つ |
| 10 | `precedent_scarcity` | 比較可能な前例の不足 | 重要条件が近い有用な前例がない |
| 11 | `option_ranking_sensitivity` | 案の順位の不安定さ | 小さな前提変更で有力案の順位が逆転する |
| 12 | `predecision_learning_potential` | 決定前の学習余地 | 期限内の調査・試行で核心的な未知を大きく減らせる |

### B. 時間・観測・変化（context: 13–20）

| # | attribute_id | 日本語名 | 100の意味 |
|---:|---|---|---|
| 13 | `decision_deadline_pressure` | 決定期限の圧力 | 必要な検討を終える時間がない |
| 14 | `delay_cost` | 延期の損失 | 遅延自体が主要目的を大きく損なう |
| 15 | `opportunity_window_transience` | 好機の短命さ | 現在の有力な選択機会が短期間で消える |
| 16 | `environment_change_rate` | 環境の変化速度 | 検討中にも重要前提が変わる |
| 17 | `benefit_realization_delay` | 便益発現までの長さ | 主便益が評価期間末またはそれ以降に現れる |
| 18 | `impact_duration` | 影響の持続期間 | 主要な影響が評価期間を超えて長く残る |
| 19 | `feedback_delay` | 結果判明の遅さ | 修正可能な時期を過ぎてから適否が分かる |
| 20 | `success_measurability` | 成功の測定可能性 | 成功条件と観測方法が具体的・定量的に定義されている |

### C. 結果と影響（context: 21–28）

| # | attribute_id | 日本語名 | 100の意味 |
|---:|---|---|---|
| 21 | `downside_severity` | 通常の不首尾による損失規模 | 不首尾が主要目的・存続を脅かす |
| 22 | `downside_likelihood` | 許容下限を割る見込み | 許容下限を下回る結果がほぼ避けられない |
| 23 | `upside_potential` | 上振れ便益の大きさ | 比較基準を大きく上回り目的達成を変える便益がある |
| 24 | `tail_risk_exposure` | 極端損害への曝露 | 破局的または上限不明の損害経路がある |
| 25 | `direct_impact_scope` | 直接影響の広さ | ほぼ全体の人・機能・拠点へ直接影響する |
| 26 | `spillover_potential` | 間接的な波及性 | 二次・三次影響が連鎖して広く伝わる |
| 27 | `risk_coupling` | 既存リスクとの連動 | 既存資産・活動と同じ要因で同時に悪化する |
| 28 | `benefit_burden_separation` | 受益者と負担者の分離 | 便益を得る者と損失・負担を負う者がほぼ別 |

### D. 拘束と選択肢（context: 29–38）

| # | attribute_id | 日本語名 | 100の意味 |
|---:|---|---|---|
| 29 | `binding_commitment_duration` | 拘束期間 | 評価期間の大半以上、義務から離脱できない |
| 30 | `reversal_cost` | 撤回の資源負担 | 撤回負担が実質的に撤回を阻む |
| 31 | `reversal_latency` | 撤回に必要な時間 | 元の運用へ戻るまでに有効な修正期間を超える |
| 32 | `residual_irreversibility` | 撤回後に残る不可逆性 | 撤回しても核心的な元の状態が復元できない |
| 33 | `exit_permission_dependence` | 離脱の他者同意依存 | 他者の承認なしでは離脱できない |
| 34 | `future_option_foreclosure` | 将来の選択肢の閉鎖 | 実行により主要な将来経路が失われる |
| 35 | `viable_alternative_scarcity` | 実行可能な代替案の不足 | 最低条件を満たす有効な代替案がない |
| 36 | `choice_exclusivity` | 選択の排他性 | 一つの案を選ぶと他案を同時に保持・併用できない |
| 37 | `trial_feasibility` | 小さく試せる度合い | 低い拘束で本格判断に有用な検証ができる |
| 38 | `commitment_divisibility` | コミットメントの分割可能性 | 小さな独立単位で拘束を追加できる |

### E. 資源と実行（context: 39–48）

| # | attribute_id | 日本語名 | 100の意味 |
|---:|---|---|---|
| 39 | `upfront_resource_burden` | 初期資源負担 | 初期に主体の主要資源を大きく占有する |
| 40 | `recurring_resource_burden` | 継続資源負担 | 維持のため恒常的に主要資源を消費する |
| 41 | `resource_slack_scarcity` | 資源余力の不足 | 資金・人員・時間・容量が既に限界に近い |
| 42 | `capability_gap` | 必要能力との隔たり | 実行に必要な核心能力が欠けている |
| 43 | `implementation_complexity` | 実行構造の複雑さ | 多数の密な依存・工程・例外がある |
| 44 | `coordination_burden` | 関係者間の調整負担 | 広範かつ継続的な調整が必要 |
| 45 | `external_dependency` | 外部依存 | 成否の核心が主体の管理外の供給・行動次第 |
| 46 | `outcome_controllability` | 結果の制御可能性 | 主体自身が主要結果を継続的に調整できる |
| 47 | `failure_detectability` | 失敗兆候の検知可能性 | 重大化前に主要な異常を早期検知できる |
| 48 | `recovery_difficulty` | 失敗後の復旧困難度 | 許容状態へ戻す現実的な手段がない |

### F. 関係者と統治（context: 49–56）

| # | attribute_id | 日本語名 | 100の意味 |
|---:|---|---|---|
| 49 | `objective_ambiguity` | 目標の未確定性 | 何を達成すべきかという核心目的が未確定 |
| 50 | `stakeholder_conflict` | 利害の対立 | 主要関係者の要求が強く衝突する |
| 51 | `authority_fragmentation` | 決定権限の分散 | 多数の主体が独立した承認・拒否権を持つ |
| 52 | `incentive_misalignment` | 誘因のずれ | 報酬・評価・責任が共有目的と逆の行動を強く促す |
| 53 | `formal_constraint_intensity` | 明文化された制約の強さ | 規則・契約・標準により許される選択が極めて狭い |
| 54 | `impact_inequality` | 影響配分の偏り | 損失・負担が少数の関係者へ強く集中する |
| 55 | `knowledge_concentration` | 重要知識の集中 | 不可欠な知識が一者に集中し代替できない |
| 56 | `decision_implementation_separation` | 決定者と実行者の分離 | 決定者と実行担当がほぼ完全に別 |

### G. 成果に関する優先基準（criterion: 57–66）

criterionの100は「他の有力な便益を譲ってでも、その基準が結論を支配した」を意味する。

| # | attribute_id | 日本語名 | 100の意味 |
|---:|---|---|---|
| 57 | `loss_avoidance_priority` | 損失回避の重視 | 通常損失の抑制が決め手 |
| 58 | `catastrophe_avoidance_priority` | 破局回避の重視 | 他の便益を捨てても極端損害経路を排除 |
| 59 | `upside_priority` | 上振れ追求の重視 | 大きな便益・飛躍の可能性が決め手 |
| 60 | `efficiency_priority` | 効率の重視 | 資源投入当たり成果の改善が決め手 |
| 61 | `liquidity_priority` | 当面の支払余力の重視 | 総効率より当面利用可能な資金の保持が決め手 |
| 62 | `long_term_value_priority` | 長期価値の重視 | 短期負担を受け入れて長期価値を優先 |
| 63 | `speed_priority` | 速さの重視 | 他の利点を譲っても開始・到達の速さを優先 |
| 64 | `performance_priority` | 到達性能の重視 | 高い機能・成果・品質水準が決め手 |
| 65 | `reliability_priority` | 安定達成の重視 | 最大性能より期待水準を継続的に満たすことを優先 |
| 66 | `robustness_priority` | 条件変化への頑健さの重視 | 幅広い条件で許容性能を保つことが決め手 |

### H. 選択と運用に関する優先基準（criterion: 67–76）

| # | attribute_id | 日本語名 | 100の意味 |
|---:|---|---|---|
| 67 | `adaptability_priority` | 変更容易性の重視 | 採用後に構成・方法を変えられることが決め手 |
| 68 | `optionality_priority` | 将来の選択肢の重視 | 当面の便益を譲ってでも有力な将来経路を残す |
| 69 | `autonomy_priority` | 自律的な制御の重視 | 効率などを譲ってでも自己管理・自己決定を優先 |
| 70 | `simplicity_priority` | 単純さの重視 | 機能などを譲っても仕組み・手順の単純さを優先 |
| 71 | `interoperability_priority` | 接続・互換性の重視 | 既存・他者の仕組みとの接続・交換可能性が決め手 |
| 72 | `scalability_priority` | 拡大への対応の重視 | 将来の量・人数・範囲の拡大余地が決め手 |
| 73 | `learning_priority` | 学習の重視 | 即時成果を譲ってでも行動から知識を得る |
| 74 | `evidence_assurance_priority` | 根拠の確かさの重視 | 十分な裏づけが揃うことを決定条件とする |
| 75 | `continuity_priority` | 継続性の重視 | 他の便益を譲っても活動・関係・体験の中断を避ける |
| 76 | `reversibility_priority` | やり直し可能性の重視 | 戻れることが判断の決め手 |

### I. 社会性と将来に関する優先基準（criterion: 77–86）

| # | attribute_id | 日本語名 | 100の意味 |
|---:|---|---|---|
| 77 | `fair_distribution_priority` | 配分の公平性の重視 | 総便益などを譲ってでも公平な配分を優先 |
| 78 | `stakeholder_acceptance_priority` | 関係者の受容の重視 | 関係者の納得・支持の確保が決め手 |
| 79 | `procedural_legitimacy_priority` | 手続き上の正当性の重視 | 結果が有利でも不適切な手続きを受け入れない |
| 80 | `compliance_priority` | 明文化された義務の遵守の重視 | 他の利点があっても規則・契約違反を受け入れない |
| 81 | `affected_party_agency_priority` | 影響を受ける人の主体性の重視 | 効率などを譲っても当事者の選択・拒否権を守る |
| 82 | `trust_reputation_priority` | 信頼・評判維持の重視 | 短期利益を譲ってでも特定相手の信頼や社会的信用を守る |
| 83 | `confidentiality_priority` | 情報保護の重視 | 利便性などを譲ってでも情報の閲覧・利用を制限する |
| 84 | `explainability_priority` | 説明・追跡可能性の重視 | 成果などを譲ってでも理由と根拠を追えることを優先 |
| 85 | `strategic_alignment_priority` | 上位方針との整合の重視 | 単独では有利でも使命・戦略に反する案を避ける |
| 86 | `capability_accumulation_priority` | 再利用できる能力の蓄積の重視 | 即時成果を譲ってでも技能・知識・仕組みを主体内に残す |

### J. 判断方法（method: 87–95）

| # | attribute_id | 日本語名 | 100の意味 |
|---:|---|---|---|
| 87 | `expected_value_use` | 期待値による判断 | 確率で重みづけた結果比較が主要ルール |
| 88 | `worst_case_use` | 最悪条件による判断 | 不利な条件での結果・最大損害が主要ルール |
| 89 | `hard_threshold_use` | 足切り条件による判断 | 他の便益で補えない最低・禁止条件が結論を決める |
| 90 | `satisficing_use` | 十分条件で探索を止める判断 | 必要十分な案を得た時点で最良案探索を止める |
| 91 | `causal_model_use` | 因果説明による判断 | 原因から結果への作用機構が主要根拠 |
| 92 | `precedent_use` | 前例による判断 | 比較可能な過去の実績・失敗が主要根拠 |
| 93 | `expert_judgment_use` | 専門家判断への依拠 | 専門家の総合判断そのものが主要根拠 |
| 94 | `scenario_analysis_use` | 複数シナリオによる判断 | 複数の将来条件にまたがる比較が判断の中心 |
| 95 | `compensatory_tradeoff_use` | 基準間の交換による判断 | 一基準の不足を別基準の便益で明示的に補う |

### K. 選んだ対応（strategy: 96–100）

| # | attribute_id | 日本語名 | 100の意味 |
|---:|---|---|---|
| 96 | `immediate_commitment_extent` | 即時に確定した範囲 | 検討していた本格行動の全範囲を直ちに確定 |
| 97 | `precommitment_learning_extent` | 本格決定前の情報取得 | 主要な未知を検証してから本格拘束へ進む対応を採用 |
| 98 | `staged_commitment_extent` | 段階的な確定 | 小さな判断点ごとに中止・追加判断できる形を採用 |
| 99 | `risk_mitigation_extent` | 予防・緩和策の組込み | 主要な損害経路へ複数の具体的対策を組み込む |
| 100 | `retained_option_extent` | 選択後に残した選択肢 | 事前に認識した有力な将来経路の大半を維持 |

## 5. 添付案からの統合判断

添付の14カテゴリ案は網羅性とアンカー記述に優れている。一方、同じ100次元の中で「検討対象の性質」「その基準を重視した程度」「実際に選んだ対応」が混在し、見送りDecisionなどで採点対象が反転する問題がある。本設計では有用な概念を残しつつ4層へ再配置した。

| 添付案の概念 | 最終判断 | 反映先・理由 |
|---|---|---|
| `information_asymmetry` | 採用 | #9。逆選択・相手側情報優位は証拠源集中とは異なり、分野横断の検索価値が高い |
| `success_measurability` | 採用 | #20。結果が出る時期や失敗検知とは別に、成功基準そのものの観測可能性を保持 |
| `choice_exclusivity` | 採用 | #36。代替案の多様さより、両取りできるか否かの方が拘束構造を直接表す |
| `reputation_weight` | 定義修正して採用 | #82。特定相手の信頼と社会的評判を同じ「信用資本への重み」として扱う |
| `cost_estimate_certainty` | 統合 | #1の現状不確実性と#4の証拠信用性で扱う。費用だけを独立させると特定情報種別が過剰代表される |
| `deliberation_duration` | ベクトルから除外 | 実測可能なメタデータとして保持。長時間検討したこと自体は判断原理ではない |
| `decision_recurrence` | ベクトルから除外 | 反復性は検索フィルタ用メタデータに適する。単発でも反復でも同じ判断原理は成立する |
| `terms_negotiability` | 統合 | #33離脱同意、#38分割可能性、#46制御可能性へ分解。交渉対象によって意味が変わる総合概念を避ける |
| `competitive_pressure` | 統合 | #14延期損失、#15好機の短命さ、#16環境変化、#50利害対立として観測可能な構造へ分解 |
| `revisit_intention` | strategyメタデータへ | `review_at` / `review_trigger` として明示保存。予定の有無を連続値へ無理に変換しない |
| `risk_tolerance` | 除外 | 恒常的性格を推測せず、#57損失回避、#58破局回避、#59上振れ追求という今回の判断上の重みへ分解 |
| `decision_confidence` | 検索属性から除外 | 記録者の自信と属性抽出器のconfidenceを混同しやすい。必要なら独立メタデータにする |
| `emotional_load` | 初期版では除外 | 記録文体への依存が強く再現性が低い。明示的な感情記録が蓄積した場合に再検討 |
| `sunk_cost_salience` | 初期版では除外 | 登場度は取れるが、既存資産の合理的活用と認知バイアスを区別しづらい |
| `analytical_vs_intuitive` | 方法へ分解 | #87〜95の実際に使った推論方法の方が再現性・説明力が高い |
| `ethical_weight` | 分解 | #77公平、#79手続き、#81当事者主体性、#82信頼、#83情報保護へ分ける |

## 6. 検索設計

### 6.1 層別類似度

100次元を一度に単純平均しない。項目数の多いcontextがcriterionを圧倒するため、まず層別に類似度を計算する。

```text
context_similarity   = similarity(attributes 1..56)
criterion_similarity = similarity(attributes 57..86)
method_similarity    = similarity(attributes 87..95)
strategy_similarity  = similarity(attributes 96..100)
```

初期のFingerprint検索例:

```text
fingerprint_similarity =
    0.30 * context_similarity
  + 0.45 * criterion_similarity
  + 0.15 * method_similarity
  + 0.10 * strategy_similarity
```

「同じ判断原理」を重視するためcriterionを最大にする。重みは固定仕様ではなく、評価用ペアデータから調整する。

### 6.2 欠損とconfidence

- 両Decisionで `applicable` かつ値がある属性だけを距離計算する
- 属性ごとのconfidenceを寄与度に使う
- 共通評価属性が少ない場合、類似度が高くても検索確度を下げる
- 類似度と別に `coverage`、`shared_attribute_count`、`effective_confidence` を返す
- 同一層内で1〜2項目しか比較できない場合、その層のスコアを確定値として扱わない

### 6.3 相関による多重計上

概念的に分離できても、実データ上で強く相関する属性は存在する。運用データで相関・分散・近傍寄与を測定し、次を行う。

- ほぼ定数になる属性は削除候補
- 常に同じ値になる属性対は統合候補
- 高相関クラスタはクラスタ全体の重みを正規化
- 人間の類似判定と逆方向に働く属性はアンカーまたは定義を修正
- schema revision時も古いFingerprintを再現できるよう定義・配置・抽出器をversion管理

### 6.4 Semantic検索との併用

SemanticとFingerprintは最初から1スコアへ潰さず、候補集合とスコアを別々に観測する。

```json
{
  "semantic_score": 0.71,
  "fingerprint_score": 0.84,
  "fingerprint_layers": {
    "context": 0.68,
    "criterion": 0.93,
    "method": 0.88,
    "strategy": 0.70
  },
  "coverage": 0.76,
  "top_matching_attributes": [],
  "top_differing_attributes": []
}
```

候補取得はSemantic上位とFingerprint上位の和集合にし、最終順位の融合方法は実利用評価から決める。

## 7. v1の合格試験

1. 同じDecisionを言い換えてもFingerprintと検索順位が大きく変わらない
2. 同じ判断原理を別分野へ移してもcriterion・methodの近さが維持される
3. 状況を固定してRationaleだけ変えるとcontextは維持されcriterionが変わる
4. 同じ結論でも理由が違えば近くなりすぎない
5. 反対の結論でも同じ原理ならcriterion上で近くなる
6. 記録を短くした場合、情報不足の属性が0ではなくunknownへ移る
7. 根拠を一つ除くと、関係属性のconfidenceまたはapplicabilityだけが適切に変わる
8. 同じ入力を繰り返し評価したとき、近傍順位を壊すほど値が揺れない
9. 人間が「同じ判断原理」と評価したペアがSemantic検索より上位に出る
10. トピックが同じだけで判断原理が異なるペアがFingerprint検索では離れる

## 8. v1で別メタデータとして保持するもの

次は有用だが、100次元へ入れない。

- decision type: approve / reject / defer / delegate / experiment など
- domain / topic
- deliberation duration
- decision recurrence
- review date / review trigger
- decision maker confidence（明示された場合のみ）
- decision outcomeと事後評価
- 金額などの生値

これらはフィルタ、説明、将来の検証に利用する。
