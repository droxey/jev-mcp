# Run the evidence server

Build and inspect the local server before creating a tunnel or changing any account settings. The recommended entry point is `cmd/jev-evidence`; the preserved `cmd/jev-mcp` primitive server has different provider/fallback behavior.

## Build and check configuration

1. From the repository root, build the evidence server:

   ```sh
   go build -o bin/jev-evidence ./cmd/jev-evidence
   ./bin/jev-evidence status
   ```

2. Inject `TYPESAFE_API_KEY` through the existing secret store or process environment. Do not paste keys into chat, source files, command arguments or logs. The server does not need another generative-service key.
3. Run `./bin/jev-evidence models` with the provider key configured; discovery does not require a selected model. Select `JEV_EVIDENCE_MODEL` explicitly from the returned IDs, preferably an immutable version when available. The server never silently selects another model/provider. A mutable alias remains observable as an alias unless the provider reports a resolved version.
4. Run `./bin/jev-evidence status` again. Status is secret-free and does not make a paid model call. It reports configuration health and requested identity; configuration alone does not establish successful provider execution.

| Setting | Purpose |
| --- | --- |
| `TYPESAFE_API_KEY` | Server-side TypeSafe bearer credential. Never send it in tool arguments or evidence. |
| `JEV_EVIDENCE_MODEL` | Explicit requested model. Verify available IDs through authenticated discovery. |
| `JEV_EVIDENCE_HTTP_TOKEN` | Independent local HTTP bearer token. Required for HTTP unless unauthenticated loopback is explicitly enabled. It is separate from provider and tunnel credentials. |
| `CONTROL_PLANE_API_KEY` | External tunnel-client runtime key for doctor/run, injected into that process. An admin key is not the tunnel runtime key. |
| `JEV_HTTP_AUTHORIZATION` | Optional tunnel HTTP configuration: the full local `Bearer …` value, injected from a secret store. Never put its value in the profile command or URL. |

Do not log environment dumps. `.env` and `.env.local` are ignored as a precaution; the CLI reads process environment and does not automatically load them.

## Run locally over stdio

Configure an MCP client with the absolute evidence-binary path and `serve --transport stdio` arguments. The client must inject provider configuration into the server process. Stdout carries newline-delimited MCP JSON; diagnostics use stderr.

```sh
./bin/jev-evidence serve --transport stdio \
  --timeout 90s --max-calls 64 --batch-size 8 --concurrency 2 --retries 0
```

One deadline covers dependent phases and retries. The call cap includes retry attempts. Keep local caps small enough for the task: the defaults are ceilings, not an instruction to consume them. The 90-second maximum stays below the tunnel's documented 120-second envelope, which includes queue time. Byte/call/deadline caps do not enforce a dollar amount. No verified provider-enforced monetary control is integrated; price-based estimates would remain estimates.

## Run locally over Streamable HTTP

1. Inject `JEV_EVIDENCE_HTTP_TOKEN`, then start:

   ```sh
   ./bin/jev-evidence serve --transport http --listen 127.0.0.1:8080
   ```

2. Connect an MCP client to `http://127.0.0.1:8080/mcp` with `Authorization: Bearer` plus the configured local token. Pass credentials through the client's secret facility, not a URL or checked-in configuration.
3. For a deliberately unauthenticated single-user loopback setup, use `--allow-unauthenticated-loopback` explicitly. Do not expose that listener through a public proxy.

The HTTP adapter is stateless and returns JSON responses using the pinned Go SDK. `GET`/`DELETE` are not session/SSE endpoints; they return 405. Clients must use normal MCP initialization and POST tool calls. No session header is required. Local peer, Host and Origin protections remain active. Forwarded headers do not confer trust or replace a loopback peer. A local reverse proxy must rewrite Host to the configured loopback listener and keep credentials separate from the incoming public request.

See [transport contracts](transports.md) for the trust boundary and proxy test expectations.

## Prepare Secure MCP Tunnel setup

These are **instructions for an account owner**, not actions performed by local development. Account/tunnel association requires explicit approval after reviewing the local build, tests and this runbook.

1. Obtain the official external client from [openai/tunnel-client releases](https://github.com/openai/tunnel-client/releases/latest). Inspect its help before using the current commands:

   ```sh
   tunnel-client help quickstart
   ```

2. Open [Platform tunnel settings](https://platform.openai.com/settings/organization/tunnels). Tunnel creation requires the Platform **Read + Manage** permissions; use/run/select requires **Read + Use**. Obtain the runtime control-plane key through the approved account workflow and inject it as `CONTROL_PLANE_API_KEY`. Do not use `OPENAI_ADMIN_KEY` as the runtime key.
3. After approval, initialize a stdio profile with the actual tunnel ID and absolute binary path:

   ```sh
   tunnel-client init --sample sample_mcp_stdio_local \
     --profile local-stdio --tunnel-id <tunnel-id> \
     --mcp-command '<absolute-path-to-evidence-binary>'
   tunnel-client doctor --profile local-stdio --explain
   tunnel-client run --profile local-stdio
   ```

   The evidence binary's default mode is stdio serving. Ensure the tunnel process inherits its server-side provider environment. Use one active stdio tunnel client per tunnel ID; stop the existing process before replacement.
4. For HTTP forwarding, replace `--mcp-command` with `--mcp-server-url http://127.0.0.1:8080/mcp`. Keep local authentication by configuring **both** forwarding/discovery headers using the external client's documented syntax:

   ```text
   --mcp.extra-headers 'Authorization: env:JEV_HTTP_AUTHORIZATION'
   --mcp.discovery-extra-headers 'Authorization: env:JEV_HTTP_AUTHORIZATION'
   ```

   Inject the full Bearer value through `JEV_HTTP_AUTHORIZATION`. The configured MCP origin is the boundary for these static headers; do not forward a credential to a different origin. Confirm the current [client configuration documentation](https://github.com/openai/tunnel-client/blob/main/docs/configuration.md) when editing profiles. The stdio path avoids a second local HTTP credential.

The current command shapes, 10 MiB envelope, 120-second absolute request limit and permission names were checked against the [official guide](https://developers.openai.com/api/docs/guides/secure-mcp-tunnels) on **2026-10-21**. This checkout does not install, initialize or associate the external client automatically. A tunnel provides the cloud-to-private path; local HTTPS alone does not.

## Connect ChatGPT after approval

1. Open [ChatGPT plugin settings](https://chatgpt.com/plugins) in the intended account/workspace. Choose **+ → Add custom MCP server → Connection: Tunnel**.
2. Select the approved tunnel or enter its `tunnel_id`, review the authentication/risk consent, create it as a plugin and install it. Platform organization permissions and ChatGPT workspace/plugin permissions are separate; a successful tunnel process does not grant ChatGPT access.
3. Call `status` from ChatGPT, then perform the small opt-in check described below. Use [ChatGPT preparation instructions](chatgpt.md) for evidence requests.

Check the current [custom MCP guide](https://developers.openai.com/api/docs/guides/custom-mcp-server) and [connect/test guide](https://developers.openai.com/plugins/deploy/connect-chatgpt) if this UI differs. Current access documentation describes **web**. For iOS, verify this account actually offers the plugin, then observe `status` and one real authenticated bounded evaluation from the mobile app. Record exact unavailable/permission text if blocked. Mobile availability has not been established by local development.

## Run opt-in live checks

Offline tests use mocked providers. Do not run paid checks or calibration sweeps automatically.

For a live check, obtain explicit approval, use one synthetic candidate, one mandatory claim and one short passage, set a 20-second deadline, `--batch-size 1 --concurrency 1 --retries 0`, and a call cap covering relation, sufficiency/directness and Choice. Use the exact opt-in command recorded in [verification](verification.md); observe billed usage only when the provider actually returns it. Never claim the call cap is a hard monetary budget.

For the authenticated ChatGPT/iOS check, use that same small fixture. Record requested/resolved model identity, date, client version, tool/report IDs, phase status, selected or abstention outcome and citations. Redact credentials. Local/mock success, live semantic accuracy and mobile transport success are separate evidence.

## Recover from outages and shut down

| Symptom | Action |
| --- | --- |
| Server unavailable locally | Restart the binary in the configured transport and call `status`. Check process exit and stderr without copying secrets. |
| Tunnel doctor fails | Confirm `CONTROL_PLANE_API_KEY` injection and Read + Use access at [Platform tunnel settings](https://platform.openai.com/settings/organization/tunnels). Verify the profile's tunnel ID and local command/URL. |
| ChatGPT cannot find/select plugin | Inspect [ChatGPT plugin settings](https://chatgpt.com/plugins) and workspace permissions. Platform access alone is insufficient. |
| iOS lacks the plugin/tool | Record the exact app/account status. Check web configuration separately; do not infer mobile support or change account scopes automatically. |
| HTTP 401/403 | Check the local bearer token, Host/Origin and loopback peer. Do not weaken protection to make a public route work. |
| Provider failure/invalid output | Check secret-free `status`, explicit model identity, provider connectivity and report phase status. Retry only within an approved bounded request. Do not present it as missing evidence. |
| Host sleeps/network disappears | The local server/tunnel becomes unavailable. Wake/reconnect the host and restart processes as needed. Availability failures are not evidence verdicts. |

Stop foreground server/tunnel processes with **Ctrl-C**. There is no invented tunnel “stop” command. Stop the old stdio tunnel process before starting another for the same tunnel ID. An always-on private host can improve availability without publishing the server. Moving to that host or changing account associations is a separate authorized operation.

## Prepare optional public HTTPS deployment

This is deployment readiness only. Do not deploy, alter firewall rules, add DNS records, expand account scopes or change authentication as part of local development.

1. Keep the Go listener private on loopback behind a TLS reverse proxy. Caddy's [automatic HTTPS](https://caddyserver.com/docs/automatic-https) can manage certificates after the owner authorizes the domain/DNS/network setup. Pin deployment artifacts and make rollback concrete before release.
2. Add an MCP-compatible **OAuth 2.1 resource-server boundary** before exposing `/mcp`. Implement or integrate protected-resource metadata, authorization-server discovery, audience/resource validation, appropriate authorization-code/PKCE flow, short-lived scoped access tokens and revocation according to current [MCP authorization](https://modelcontextprotocol.io/specification/latest/basic/authorization) and [OpenAI authentication](https://developers.openai.com/plugins/build/auth) requirements. The current local bearer-token adapter does **not** implement public MCP OAuth.
3. Terminate public OAuth outside the private listener, validate each request, then inject a separate local bearer token. Strip incoming forwarding/authentication headers before adding trusted internal values; rewrite Host explicitly. Never pass the TypeSafe or tunnel runtime key to callers.
4. Limit request bodies, concurrency, timeouts and response buffering to the supported Streamable HTTP contract. Verify initialization, tool calls, auth discovery, denied origins, forwarding-header spoofing and cancellation through the actual proxy before publishing.

Public deployment blocker: **no OAuth resource gateway is implemented**. Production HTTPS/OAuth, public network access and account-specific ChatGPT/iOS behavior remain unverified until separately implemented and tested. Public plugin submission through a Secure MCP Tunnel is not supported by the reviewed tunnel workflow.
