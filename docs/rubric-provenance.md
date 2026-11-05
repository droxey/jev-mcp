# Read an evidence report

Read the outcome, candidate eligibility and missing/conflicting requirements before using a selected candidate. A model judgment evaluates the supplied material; it does not establish empirical truth. All uncalibrated judgments are advisory.

## Interpret documentary-support-v1

The Score question evaluates **only the supporting passage subset identified by the earlier relation pass**. Counterevidence remains visible independently.

| Semantic level | Complete operational anchor |
| --- | --- |
| **0 — None** | No valid, relevant supporting passage establishes a material part of the claim. A URL alone, an off-topic passage, or a mismatched subject does not qualify. |
| **1 — Partial or conditional** | Evidence supports some of the claim, but at least one indispensable component or premise remains unestablished. An explicitly disclosed assumption remains an assumption. |
| **2 — Complete** | Every material component and required premise has cited support at the claimed scope. Any derivation is explicit and checkable. |

Directness is separate: a complete, checkable derivation can qualify for level 2, while a direct assertion can be unreliable. A proposed complete classification is rejected by Go policy when required supporting references are missing or an indispensable premise lacks accepted support. The report retains the model's raw classification and the rejection reason. Quote checks establish that supplied bytes match; they do not prove the passage supports the claim.

The semantic labels 0/1/2 are not an assumption about arbitrary provider wire values. The adapter validates the requested ordered criteria against the returned legend. Native Score is a probability-weighted mean and may be fractional. The reported semantic level is the uniquely most probable anchor in the distribution; a tie leaves the level unresolved. This is an advisory classification, not a calibrated accuracy cutoff. Keep the raw score, legend, distribution and actual provider confidence. Never round `1.8` to “Complete,” or turn a probability into certainty. Go applies the independent reference, premise and conflict gates before accepting any proposed Complete classification.

A valid level-0/1 judgment or a Go policy rejection is still a successfully executed judgment. Cancellation, skipped items and exhausted budgets produce an incomplete execution outcome; they are not model verdicts. Invalid provider output and provider failure remain separate operational outcomes.

## Keep the dimensions separate

| Dimension | What to inspect |
| --- | --- |
| Relation | Each claim/passage pair has `supports`, `contradicts`, `insufficient_information`, or `ambiguous`. Preserve both supporting and contradicting IDs; any support must not erase a contradiction. |
| Documentary sufficiency | Apply the ordered anchors to the supporting subset, then check all indispensable premises and required references in Go. |
| Directness | `direct_statement`, `explicit_derivation`, `mixed`, or `unknown`; do not derive this from the support level. |
| Coverage | Count required claims/premises evaluated, supported, contradicted, missing and skipped. Failed/skipped work is not a completed level-0 judgment. |
| Source quality | Inspect declared provenance, snapshot availability, relevance, authority, observation/artifact/calculation versus assertion, and methods appropriate to the task. Metadata remains unverified unless independently checked. |
| Independence | Inspect original-source groups, copied/syndicated material and declared dependencies. Count one common origin once; unknown independence remains unknown. |
| Applicability | Match entity, population, definition, version, jurisdiction, conditions and relevant time before treating a passage as support or contradiction. |
| Model uncertainty | Preserve raw primitive values and distributions. Selected-option probability and provider confidence are separate. Noul is proposition probability and has no separate native confidence field. |
| Execution validity | Inspect phase/item status and the top-level provider outcome. Successful evidence judgments, incomplete/cancelled work, invalid responses and provider failures have different meanings. |

An aligned contradiction expresses incompatible propositions about the same material subject, scope and relevant time. “Version 1 lacks feature X” and “Version 2 has feature X” are not a contradiction when evaluating version 2. Different population, time, definition or conditions can explain apparent disagreement. Cherry-picking means selectively presenting evidence or omitting relevant material; it can occur without logical contradiction. This server cannot detect evidence that was never supplied.

## Supply auditable provenance

1. Assign globally unique stable IDs using 1–128 ASCII letters/digits, dots, underscores or hyphens. Reuse an ID only for the same record identity; input/source hashes detect content changes. Go computes omitted source hashes and rejects any supplied hash that does not match.
2. Supply the snapshot text and exact UTF-8 byte offsets: `start_byte` inclusive and `end_byte` exclusive. `excerpt` must exactly equal that range. URLs and titles identify sources but never replace the snapshot.
3. Supply publication, effective and retrieval dates when known. Source `captured_at`/`published_at` accept YYYY-MM-DD or RFC3339 without inventing finer precision; applicability intervals and `as_of` use YYYY-MM-DD. Do not invent dates, versions, authority, expertise or independent origins. Omitted metadata remains unknown.
4. Declare a common `origin` for syndication/copying and `derived_from` source IDs where known. Two domains can repeat one original source; different domain names do not establish independence. Use the explicit unknown-origin representation accepted by validation when independence is unknown.
5. Select a versioned task profile and explicit facets. A software-release decision needs version and artifact compatibility; a population study needs relevant population, methods and observation dates. Do not infer authority or source quality from the domain name.

The server accepts snapshots/excerpts supplied by ChatGPT or the caller. Retrieval, credentialed source access and source verification happen outside the server. Do not place passwords, tokens or unnecessary personal data in a snapshot: evidence is sent to TypeSafe for judgment.

`lint_plan.source_hashes` returns computed snapshot hashes. Evaluation reports retain checked `citations`, source records in `sources`, observed provider identities in `models`, and the original input's `plan_sha256`. A model-identity change flags calibration; it does not silently choose a substitute provider or model.

## Explain the policy outcome

Mandatory requirements must have complete accepted support for all mapped claims and premises, with no unresolved applicable counterevidence. Preferences cannot compensate for a mandatory failure. Only eligible candidate IDs reach the dependent Choice question. The server validates the selected ID and exposes investigation/abstention; it never silently substitutes a runner-up after a bad selection.

An applicable `ambiguous` relation produces `unresolved_evidence_relation` and blocks policy Complete until resolved. An extra `insufficient_information` passage does not invalidate otherwise complete support; it also supplies no support of its own.

Use qualifications with their scope intact. For example:

> Complete documentary support; one original source; reliability not independently assessed; no conflict found in the supplied material.

That statement says nothing about unsupplied sources or empirical accuracy. If new factual claims are added while explaining the result, evaluate them first or remove them. See [ChatGPT instructions](chatgpt.md) for the preparation and final-answer workflow.

## Compare model runs cautiously

Record requested model, observed resolved model/version when supplied, rubric, policy and profile versions beside calibration results. An immutable model identifier is preferable when provider-supported. If only a mutable alias is observable, disclose it and flag calibration as potentially stale after a model-identity change. There is no silent model/provider fallback in the evidence entry point.

Use original labeled fixtures, keep tuning cases separate from held-out reporting, and report false support, missed contradictions, false conflict, abstention and coverage. Threshold sweeps explore tradeoffs; they do not establish calibrated production cutoffs by themselves. See the [calibration guide](calibration.md).
