# Connect an MCP client

Use `cmd/jev-evidence` for `lint_plan`, `evaluate_plan` and `status`. Both adapters share one typed Go core and the official Go MCP SDK pinned in `go.mod`. A tool's plan argument is the Plan object itself, without an extra `plan` wrapper.

## Select a local adapter

| Adapter | Start command | Contract |
| --- | --- | --- |
| stdio | `./bin/jev-evidence serve --transport stdio` | Default, including when the binary has no arguments. Newline-delimited JSON on stdout; diagnostics on stderr. The launching client's OS/process access is the local authentication boundary. |
| Streamable HTTP | `./bin/jev-evidence serve --transport http --listen 127.0.0.1:8080` | POST `/mcp`, stateless JSON responses, normal initialization, no required session header. `GET` and `DELETE` return 405. Requires independent `JEV_EVIDENCE_HTTP_TOKEN` bearer authentication unless unauthenticated loopback is explicitly enabled. |

`status` and `lint_plan` are offline. `evaluate_plan` sends supplied evidence to TypeSafe only when credentials/model are configured and the plan passes validation. Responses retain structured report data and a JSON text representation for clients that consume text. Provider failures and invalid output must remain distinct from evidence outcomes.

The server does not implement URL retrieval, filesystem source retrieval, OAuth authorization-server behavior, hosted tunnels or public TLS. The developer CLI's explicit `lint --input` file is a local developer operation; MCP source URIs never become file reads.

## Preserve local HTTP protections

1. Bind a literal loopback address. Do not bind `0.0.0.0` or expose this local-token listener directly to the Internet.
2. Send the configured local bearer token through the MCP client's secret facility. Do not reuse the TypeSafe key or tunnel control-plane key.
3. Preserve loopback peer, Host and Origin validation. Spoofed `Forwarded`/`X-Forwarded-*` headers do not make a remote peer local and do not override Host/Origin checks.
4. A trusted proxy on the same host must connect to loopback, rewrite Host for that target and inject a separate local token after validating external authentication. Do not pass incoming auth or forwarding values through unchecked.
5. Keep proxy body limits and timeouts compatible with the server's one end-to-end deadline. Cancel disconnected requests so provider work can stop through context cancellation.

Transport tests cover ordinary SDK-client initialization, `status` and lint calls, stateless behavior, rejected hosts/origins/peers, bearer failure and forwarding-header spoofing. Mocked evaluation tests exercise the dependent pipeline through both adapters. These checks establish local contracts, not a production proxy or authenticated ChatGPT connection.

## Keep tunnel and public routes separate

Secure MCP Tunnel uses the official external client to bridge cloud requests to this local server. Prefer stdio; HTTP forwarding additionally needs local HTTP authentication and origin/Host compatibility. Platform tunnel permissions, ChatGPT plugin permissions, server authentication and provider authentication are separate gates. See [operations](operations.md) for current verified commands and account-owner setup.

An optional public endpoint needs TLS and compatible MCP OAuth 2.1 authorization before exposure. The existing local bearer token does not satisfy that public authentication contract. Reverse-proxy and OAuth guidance in the runbook is readiness work; deployment and account changes remain separate actions.

## Recognize incomplete access evidence

A local SDK-client round trip proves local adapter behavior. A running tunnel process proves neither ChatGPT permission nor mobile plugin availability. A successful web call proves neither account-specific iOS access nor semantic model accuracy. Record actual observed results separately; never convert a server/tunnel outage into a support level.
