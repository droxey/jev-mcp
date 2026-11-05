# Reproduce the verified local build

All local checks below passed on **2026-10-21** and were repeated successfully on **2026-11-05** for publication preparation. There are no unresolved local verification errors. Live provider, tunnel, authenticated ChatGPT and iOS checks were not performed.

The verified repository root is `$REPO`, on local branch `feat/evidence-server`, derived from public upstream `tphakala/jev-mcp` revision `f51a9267f74a641f44f90fa5e1a8d3125edc9a35`. The original **2026-10-21** handoff was uncommitted and unpublished, with no hosted fork, remote change, deployment or account association. That is historical handoff status; the November verification establishes local readiness, not a hosted push or merge. [Source review](source-review.md) retains its original source/license verification date and selected revisions.

## Use compatible verification tools

The tested toolchain is Go **1.27.2** on `darwin/arm64`, Task **3.54.0**, golangci-lint **2.15.0**, and the official Go MCP SDK **v1.8.0** pinned in `go.mod`. Task and the linter are available in the ignored project directory `.tools/bin`.

Task was installed locally with:

```sh
GOBIN="$PWD/.tools/bin" go install github.com/go-task/task/v3/cmd/task@v3.54.0
```

The existing `/opt/homebrew/bin/golangci-lint` binary was verified as v2.15.0, built with Go 1.27.2, and copied into `.tools/bin`. The inherited v2.13.2 pin failed on the Go 1.27.2 export format before analyzing the code:

```text
cannot decode "cmp", export data version 5 is greater than maximum supported version 4
```

`.golangci-version` now pins **v2.15.0**. The strict lint configuration was preserved. Put the local tools on `PATH` before repeating the gate:

```sh
export PATH="$PWD/.tools/bin:$PATH"
```

## Repeat the passing checks

These exact commands were executed successfully on the final code. Live opt-in was explicitly removed from test processes so inherited environment settings could not trigger a paid check. Run these commands serially: the ruleguard probe and lint gate share golangci-lint's process lock.

| Tested command | Observed result |
| --- | --- |
| `gofmt -l .` | Passed; no files listed. |
| `env -u JEV_EVIDENCE_LIVE go test ./...` | All packages passed. |
| `env -u JEV_EVIDENCE_LIVE go test -race ./...` | All packages passed; no race reports. |
| `go vet ./...` | Passed without diagnostics. |
| `go build ./...` | Passed. |
| `PATH="$PWD/.tools/bin:$PATH" env -u JEV_EVIDENCE_LIVE task check` | Passed the full repository gate; strict lint reported **0 issues**. |
| `git diff --check` | Passed. |

`task check` also passed build, vet on both amd64 and arm64, tagged ruleguard vet, `go mod tidy -diff`, `go mod verify`, lint configuration validation, strict lint, the required ruleguard probe test and shuffled race tests. All modules verified; no module-file updates were needed.

On **2026-11-05**, CodeRabbit CLI **0.7.6** completed `coderabbit review --agent --uncommitted --include-untracked --base main` from the repository root with exit code 0, status `review_completed`, **0 findings** and all **44 files** in that review scope listed as reviewed. Subsequent verification prose, the Grok Bot handoff and its README/project-instruction pointers received separate independent documentation review on **2026-11-05**, with no blockers. No executable code changed after the CodeRabbit snapshot.

Mocked tests cover relation-before-sufficiency ordering, supporting-subset construction, Go eligibility before Choice, mixed evidence, missing premises, copied origins, scope/time mismatches, exact citations, malformed/partial provider output, model identity, cancellation, retries and resource exhaustion. Both stdio and stateless HTTP exercise the actual TypeSafe adapter against a local HTTP mock. HTTP security checks cover local authentication, Host/Origin/peer rejection and untrusted forwarding headers. Queued cancellation retains audit records without consuming a slot or provider call. Credential-reflection tests reject secrets in provider errors and model identities.

On **2026-11-05**, the required mutation check disabled mandatory-requirement rejection in `gateCandidates`. `env -u JEV_EVIDENCE_LIVE go test ./internal/evidence -run '^TestMixedEvidencePreservedAndMandatoryConflictBlocks$' -count=1` compiled and failed because conflicting mandatory evidence incorrectly left the candidate eligible and selected. Restoring the exact reviewed policy bytes made the same test pass; the restored file and staged blob both hashed to `588bca906724125cbbd9f47c392e13fd8ce74fc4`. Formatting and staged/working-tree whitespace checks also passed after restoration.

## Run the passing CLI smoke checks

```sh
go build -o bin/jev-evidence ./cmd/jev-evidence
./bin/jev-evidence status
./bin/jev-evidence lint --input testdata/evidence/plan.json
go run ./cmd/jev-calibrate --input testdata/evidence/calibration.json
```

All four commands passed. Status reported `provider_configured:false`, the three tool capabilities and the current schema/rubric/policy versions. Fixture lint returned `valid:true`, no issues and a computed source hash.

The offline calibration report contained six held-out cases and one excluded training case. Both complete-evidence-group cases passed. Its deliberately incorrect negative-control report produced one false-support classification, one missed contradiction, one acceptance error and one abstention error; these are expected fixture diagnostics. Coverage was 1, with zero missed-support classifications, zero false conflicts and zero incomplete evidence groups. These counts validate reporting behavior, not live model accuracy or a production cutoff.

Local documentation links resolved. Changed/new files were reviewed for scope, licensing and credentials; the targeted private-key/token-pattern scan found no matches. The Apache-2.0 license and upstream notices remain. Credential configuration was checked for presence only: `TYPESAFE_API_KEY`, `JEV_EVIDENCE_MODEL`, `JEV_EVIDENCE_HTTP_TOKEN` and `CONTROL_PLANE_API_KEY` were absent. No credential values were printed.

## Keep live proof separate

| Evidence class | Current result and remaining proof |
| --- | --- |
| Source review | Contents/licenses inspected at the revisions and date in [source review](source-review.md). |
| Deterministic correctness | Local mocked policy/provider/transport checks and the full repository gate passed. |
| Live semantic accuracy | Not measured. Requires a separately approved provider run and suitable held-out analysis. |
| ChatGPT access | Not tested. Requires approved tunnel association and an authenticated ChatGPT tool call. |
| ChatGPT iOS | Not tested. Requires account-specific mobile availability and a real authenticated mobile round trip. Web configuration is insufficient. |
| Public deployment | Not performed. Public OAuth/TLS/proxy implementation and qualification remain separate work. |

The disabled live harness was checked with `JEV_EVIDENCE_LIVE=0 go test ./internal/typesafe -run '^TestLiveProviderContract$' -count=1 -v`; it correctly skipped without making a provider call.

After separate authorization and credential injection, the opt-in paid-capable command is:

```sh
JEV_EVIDENCE_LIVE=1 go test ./internal/typesafe -run TestLiveProviderContract -count=1
```

That enabled command was **not executed**. It sends one primitive question under a 45-second total timeout with no retries. Calls/bytes/time are bounded; no verified monetary hard cap is integrated. A passing smoke test would not establish general semantic accuracy. Follow [operations](operations.md) for exact setup/settings links and the separately approved ChatGPT/iOS check; record server/tunnel availability, permissions, provider execution and evidence outcomes independently.
