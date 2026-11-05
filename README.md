# Evaluate supplied evidence with ChatGPT and Jev

Build the local evidence server and inspect the original fixtures:

```sh
go build -o bin/jev-evidence ./cmd/jev-evidence
./bin/jev-evidence status
./bin/jev-evidence lint --input testdata/evidence/plan.json
go run ./cmd/jev-calibrate --input testdata/evidence/calibration.json
```

Requires Go **1.27 or later**. These commands are offline. Live evaluation requires server-side `TYPESAFE_API_KEY` and explicit `JEV_EVIDENCE_MODEL`; inject secrets from the environment or an existing secret store. Never put keys in ChatGPT messages or source passages.

## Use the evidence workflow

1. Give ChatGPT an ordinary request, candidates and supplied source snapshots. Use the [preparation instructions](docs/chatgpt.md) to produce requirements, preferences, atomic claims, indispensable premises, exact passages and assumptions.
2. Call `lint_plan`, then `evaluate_plan`. ChatGPT prepares and explains; Go validates and applies policy; TypeSafe/Jev judges narrow evidence questions. Source retrieval stays outside the server.
3. Read the report's citations, raw judgments/distributions, counterevidence, assumptions, gaps, coverage and eligibility. Only eligible candidates reach the dependent preference Choice. Investigation and abstention remain explicit.

Documentary support measures supplied material. It does not establish empirical truth, source reliability, semantic accuracy or complete retrieval. A useful qualification is: “Complete documentary support; one original source; reliability not independently assessed; no conflict found in the supplied material.” Uncalibrated judgments are advisory.

## Connect a local MCP client

```sh
./bin/jev-evidence serve --transport stdio
```

Stdio is the default, including with no arguments; diagnostics use stderr. For loopback Streamable HTTP, inject independent `JEV_EVIDENCE_HTTP_TOKEN` and run:

```sh
./bin/jev-evidence serve --transport http --listen 127.0.0.1:8080
```

Connect to `/mcp` using that local bearer token. The stateless adapter retains Host, Origin and loopback protection. See [transports](docs/transports.md) and [operations](docs/operations.md) for proxying, secrets, outages and shutdown.

The default ChatGPT path is the official **external OpenAI Secure MCP Tunnel client** forwarding stdio or loopback HTTP. The runbook contains verified commands and account-owner setup. No account/tunnel association, public deployment or firewall change is performed by this checkout. Current OpenAI setup documentation describes web access; **ChatGPT iOS** availability and a real authenticated mobile round trip remain separate gates.

## Inspect tools and policy

| Tool | Behavior |
| --- | --- |
| `lint_plan` | Offline IDs/linkage/versions/hashes/quotes/bounds validation and bounded heuristic warnings. Lint cannot prove semantic completeness or non-overlap. |
| `evaluate_plan` | Relations first, sufficiency on identified supporting subsets next, Go eligibility gates, dependent Choice. Preferences cannot compensate for mandatory gaps/conflicts. |
| `status` | Secret-free local capabilities, configuration health, requested provider/model and rubric/policy versions. No paid probe. |

The evidence entry point uses direct TypeSafe behind a narrow interface: one end-to-end deadline; bounded input, concurrency, calls and retries; no silent model/provider fallback. Raw fractional Score values are never rounded into Complete support. Supplied source instructions are untrusted evidence. URLs/paths in plans are metadata and are never fetched/opened.

## Read the project documents

| Task | Document |
| --- | --- |
| Understand dependency order and implementation plan | [Design](docs/design.md) |
| Interpret levels, applicability and accepted references | [Rubric and provenance](docs/rubric-provenance.md) |
| Prepare requests and explain citations/gaps | [ChatGPT instructions](docs/chatgpt.md) |
| Run locally, prepare tunnels or optional public OAuth/TLS | [Operations](docs/operations.md) |
| Give Grok Bot a future private CapRover deployment task for bots.ac | [Grok Bot handoff](docs/grokbot-caprover.md) |
| Check adapters and proxy trust boundaries | [Transports](docs/transports.md) |
| Analyze held-out reports and threshold sweeps | [Calibration](docs/calibration.md) |
| Inspect source contents, pinned revisions and licenses | [Source review](docs/source-review.md) |
| Reproduce checks and identify missing proof | [Verification](docs/verification.md) |

Original [testdata/evidence](testdata/evidence) fixtures are synthetic software tests, not copied research datasets or measured live semantic benchmarks.

## Verify changes

```sh
gofmt -l .
go test ./...
go test -race ./...
go vet ./...
go build ./...
task check
```

The upstream gate also checks both architectures, tidy, pinned lint and ruleguard probes. Follow [CONTRIBUTING.md](CONTRIBUTING.md) and [AGENTS.md](AGENTS.md). [Verification](docs/verification.md) records actual results separately from checks requiring credentials, account permissions or an iOS session. Paid checks and benchmark sweeps are opt-in.

## Preserve the foundation

This local derivative uses [tphakala/jev-mcp at f51a9267f74a641f44f90fa5e1a8d3125edc9a35](https://github.com/tphakala/jev-mcp/tree/f51a9267f74a641f44f90fa5e1a8d3125edc9a35). The Apache-2.0 [LICENSE](LICENSE), source notices and upstream contribution/security guidance remain.

The original `cmd/jev-mcp` primitive server retains `jev_evaluate`, provider options and fallback behavior; see its [pinned upstream README](https://github.com/tphakala/jev-mcp/blob/f51a9267f74a641f44f90fa5e1a8d3125edc9a35/README.md). Those primitives do not enforce this evidence workflow's gates. Build **`cmd/jev-evidence`** for this project's evidence contract.
