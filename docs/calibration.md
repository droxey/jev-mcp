# Inspect calibration before setting thresholds

Run the calibration CLI on **recorded reports and original labeled fixtures**. It performs no provider calls, selects no production cutoff and makes no paid sweep.

```sh
go run ./cmd/jev-calibrate --input testdata/evidence/calibration.json
```

Use the example file as the input schema. Preserve provider/requested/resolved model identity, rubric and policy versions in every run. The CLI rejects a recorded report whose `version`, `rubric` or `policy` differs from the supported versions, even when the dataset header is current. Report held-out cases separately from tuning cases. A model alias without an observed immutable resolution is a limitation; `identity_changed` flags mixed observed identities rather than implying calibration transfers between them.

The bundled synthetic reports intentionally include misses so the error accounting can be inspected. Their threshold strengths can be unknown; `unknown_strength` means no applicable numeric observation was available, not a calibrated zero probability. These fixtures do not establish live provider accuracy.

## Interpret the report

| Measure | Meaning |
| --- | --- |
| `false_support` | The observed documentary `level` is Complete while the gold documentary level is below Complete. This measures classification, independently of whether Go accepted the claim. |
| `missed_support` | The observed documentary level is below Complete while the gold level is Complete. Missing levels remain execution/coverage gaps rather than fabricated classifications. |
| `acceptance_errors` | The Go `complete` acceptance differs from `expected_accepted_complete`. When that label is omitted, the default expects acceptance only for gold Complete without contradiction or conflict. |
| `missed_contradictions` | A gold contradiction has no observed `contradicting_passage_ids`. Pure contradiction counts even without mixed support/conflict. |
| `false_conflicts` | The observed mixed-evidence conflict is true while the gold conflict is false. |
| `abstentions` / `abstention_errors` | Count outcomes other than `selected`, then compare that result with the explicit expected abstention. Evidence gaps and operational outcomes remain distinguishable in each recorded report. |
| `coverage` | Fraction of labeled claims with successful evaluation status; skipped/failed work does not become a zero-support judgment. |
| `complete_group_evaluated` / `complete_group_correct` / `incomplete_evidence_groups` | Count labeled claims with alternative evidence groups, those whose supporting IDs contain a whole group, and those missing every complete group. |
| `threshold_sweeps` | Compare each proposed cutoff with the recorded probability of the Complete documentary anchor and gold documentary level. This does not measure Go acceptance or update policy. Missing applicable probabilities increment `unknown_strength`. |
| `worst_misses` | Retain case/claim IDs and available strengths, ordered with false support and missed contradictions first. |

Complete-evidence fixtures can declare alternative sufficient passage groups. A correct fixture result requires a complete group, not a subset. Runtime evidence is open world and has no exhaustive gold group, so fixture evidence recall cannot establish that runtime retrieval found all relevant evidence.

## Extend the original fixture set

1. Write synthetic snapshots and labels yourself. Do not copy AVeriTeC research data, SciFact annotations/corpus, or other restricted benchmark records into this repository.
2. Cover both support and contradiction, copied origins, missing premises, invalid quotes, scope/date mismatches, checkable derivations and operational failures. Hold back cases before any prompt/rubric tuning.
3. Review labels independently, record disagreements, and report sample size and split. A small synthetic fixture set validates software behavior and reporting; it does not establish broad live semantic accuracy.
4. Collect live judgments only through a separately approved bounded run. Preserve raw score/distributions; never round weighted Score into an accepted Complete label.

The source review explains which research semantics informed the original implementation and which entailment-first aggregation rules were rejected. All current semantic judgments remain advisory until a suitable held-out evaluation supports a narrower claim.
