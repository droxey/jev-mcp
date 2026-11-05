# Deploy through Grok Bot on CapRover

At the next authorized Grok Bot launch, give it this task:

```text
Read AGENTS.md and docs/grokbot-caprover.md in droxey/jev-mcp. Implement and
locally test the missing production packaging, then deploy this evidence MCP
server as a private CapRover service on bots.ac using the already authorized
account and tunnel. Follow the stdio tunnel design, secret handling, single-
instance lifecycle and verification gates in that handoff. Complete deployment
within the permissions granted for this launch; request permission only for
additional scopes, tunnel/account associations, firewall changes or paid tests.
Report the deployed commit/image, health, rollback and remaining ChatGPT/iPhone
proof. Do not claim mobile success without the real authenticated tool calls.
```

This is a **future launch handoff**, not a deployment performed by this repository. No automatic external launch-task injection is wired. Attach the text above to Grok Bot's launch task; the project instructions point here when deployment is requested.

## Confirm the source and target

1. Use the intended published repository `https://github.com/droxey/jev-mcp`; verify that it exists, its visibility and the selected branch/commit before cloning or deploying. Publication is coordinated separately. Preserve the Apache-2.0 attribution to `tphakala/jev-mcp` at `f51a9267f74a641f44f90fa5e1a8d3125edc9a35`.
2. Read [README](../README.md), [operations](operations.md), [transport contracts](transports.md) and [verification](verification.md). `cmd/jev-evidence` implements supplied-evidence linting, relations, documentary sufficiency, deterministic eligibility and dependent Choice. `cmd/jev-mcp` is the legacy primitive server.
3. Verify the existing authorized CapRover connection for host **bots.ac**, its actual dashboard URL, target app and deployment permissions. The host name alone establishes neither dashboard access nor a public MCP endpoint. Do not add/repoint repository remotes or change account/tunnel associations as an incidental setup step.

## Implement the missing private-service packaging

The current root `Dockerfile` builds and launches **`cmd/jev-mcp`**, so it does not package this evidence workflow. CapRover packaging is not implemented or tested yet.

Build a dedicated image containing:

- The Go evidence binary built from `./cmd/jev-evidence` at the selected commit.
- A pinned official [OpenAI tunnel-client release](https://github.com/openai/tunnel-client/releases), with the matching platform/architecture, license and any companion executable/manifest required by that release. Keep bundled assets together; do not substitute an unrelated tunnel client or assume the legacy distroless image supports a shell launcher.
- A tested launcher, CA roots and a writable nonsecret profile/runtime directory for the nonroot user. Run the tunnel client in the foreground as the supervised parent of the evidence stdio child. Handle termination and child cleanup without orphan processes.

Record the selected client version and image digest. Check official release checksums/signatures when provided; record any unavailable verification rather than inventing it. Tighten the Docker build context first: exclude `.env*`, secret/config exports, `.tools`, `.git`, local runtime state and unrelated artifacts. Never bake keys into build arguments, layers, profiles or logs. Keep required licenses/notices in the image.

Create `deploy/caprover/Dockerfile` and a root `captain-definition` during that future implementation. The definition selects exactly one build/image source; for that Dockerfile the current schema is:

```json
{"schemaVersion":2,"dockerfilePath":"./deploy/caprover/Dockerfile"}
```

This is a proposed file to implement and validate, not a file already tested in this checkout. Source-archive deployment through CapRover avoids requiring a new image registry; use a prebuilt image only when its registry and publication are already authorized. See [Captain Definition](https://caprover.com/docs/deployments/captain-definition) and [deployment methods](https://caprover.com/docs/deployments).

## Keep the service private and singular

In the target CapRover app, select **Do not expose as web app**, configure one instance and publish no container ports. Run the tunnel health/admin listener on container loopback. The evidence child uses stdio and needs no inbound web route.

One active stdio tunnel client is permitted per tunnel ID, including during deployment and rollback. One replica alone does not prevent rolling-update overlap. Set both update and rollback to **stop-first** and verify the resulting Docker service settings before activation. CapRover's [Service Update Override](https://caprover.com/docs/service-update-override) accepts the relevant Docker service fields:

```yaml
UpdateConfig:
  Parallelism: 1
  Order: stop-first
RollbackConfig:
  Parallelism: 1
  Order: stop-first
```

Stop the old runtime before its replacement starts; accept this brief outage. Do not run a local test container against the same live tunnel while production is active. An independent test needs a separately approved tunnel ID. Retain existing egress policy; report a blocked destination rather than changing firewall rules.

No persistent evidence volume is required: callers supply snapshots per request, and this server does not store an evidence database. Treat tunnel profiles and deployment configuration separately from evidence storage; keep a nonsecret reproducible profile or recreate it using the approved existing association. Do not add a persistent volume merely to influence CapRover's rollout order.

## Configure runtime secrets and limits

Use existing approved secret injection into the CapRover service. CapRover runtime variables and backups can contain secrets; restrict access and keep their values out of chat, source, deployment archives and diagnostics. Image rollback does not roll back environment-variable values. See [application settings](https://caprover.com/docs/applications).

| Variable or launcher setting | Required handling |
| --- | --- |
| `TYPESAFE_API_KEY` | Server-side TypeSafe credential from the approved secret store. Never pass it in MCP arguments. |
| `JEV_EVIDENCE_MODEL` | Explicit model ID; prefer an immutable supported version. Record requested/resolved identity; no silent fallback. |
| `CONTROL_PLANE_API_KEY` | Existing approved tunnel runtime key. Never use an admin key as the long-running runtime credential. |
| Tunnel ID/profile | Use the approved association. A missing association or expanded scope requires owner approval at [Platform tunnels](https://platform.openai.com/settings/organization/tunnels); runtime keys are managed at [Platform API keys](https://platform.openai.com/settings/organization/api-keys). |
| `HEALTH_LISTEN_ADDR` | For example `127.0.0.1:8081`, reachable only within the container. Do not expose the admin UI through CapRover. |
| `MCP_STDIO_FAILURE_POLICY` | `exit` is supported by the reviewed client for an external supervisor to restart the whole runtime. Verify support in the pinned release before using it. |
| Evidence launcher | Execute `/app/jev-evidence serve --transport stdio --timeout 90s --max-calls 16 --batch-size 4 --concurrency 1 --retries 0`, or documented smaller caps fitting the task. Calls include retries; time/call caps do not enforce a dollar budget. |

The `/app` paths are proposed image paths, not host-home paths. Configure the tunnel's MCP command to the launcher path so its inherited environment and fixed evidence limits reach the child. Do not assume a profile-directory default in a nonroot container.

Current official setup command shapes are:

```sh
tunnel-client --version
tunnel-client help quickstart
tunnel-client init --sample sample_mcp_stdio_local \
  --profile caprover-evidence --tunnel-id <approved-tunnel-id> \
  --mcp-command /app/start-evidence
tunnel-client doctor --profile caprover-evidence --explain
tunnel-client run --profile caprover-evidence
```

Only initialize against an already approved association, or obtain approval first. Inspect the resulting profile for secret references before including it in an artifact. Do not run `init` as an unreviewed build step. `run` is the foreground container process, not a detached `nohup` task. There is no invented tunnel `stop` command. The [official client README](https://github.com/openai/tunnel-client/blob/fb43d565373123433b02b4972e734514e9f37875/README.md) and [configuration](https://github.com/openai/tunnel-client/blob/fb43d565373123433b02b4972e734514e9f37875/docs/configuration.md) were rechecked on **2026-11-05**; recheck the selected release's help at deployment time.

## Validate locally before deploying

1. Run the repository-required checks in [AGENTS](../AGENTS.md) and the passing offline fixture/CLI commands in [verification](verification.md). Packaging changes need image-specific validation in addition to existing Go tests.
2. Build the proposed image from the repository root. Verify nonroot execution, the intended evidence binary, the client version/architecture, included release assets and the absence of secrets. Test `status` and fixture lint without provider credentials or paid calls.
3. Exercise MCP initialization and `status` over the packaged stdio process with an offline client/mock. Verify that the tunnel launcher selects `jev-evidence`, its limits are applied, stderr is diagnostic-only and stdout remains the MCP stream.
4. Test TERM/restart/shutdown behavior and a failed child without a live tunnel association. Confirm the parent and child exit together, no duplicate runtime survives, and unavailable service is reported as an outage rather than an evidence verdict. Add a container-local healthcheck using the pinned client's documented health route/command; process existence alone is insufficient readiness.
5. Record the tested commit, image digest, commands, expected stop-first outage and rollback settings. Deploy through the existing approved CapRover task only after these checks pass. Do not run paid provider checks automatically.

## Verify the deployed service in layers

1. Confirm one running task, the expected image/commit, loopback-only health and no public port/domain route. Check secret-safe CapRover service logs. The documented container-local command shape is `tunnel-client health --url http://127.0.0.1:8081 --json`; verify it against the pinned release and require healthy/ready results.
2. Run tunnel doctor/readiness through the approved profile, then observe a real ChatGPT `status` call. Distinguish server, tunnel, permissions and provider configuration failures; do not call an offline `status` result a cloud round trip.
3. The operator reports iPhone developer mode enabled as of 2026-11-05; the mode type and ChatGPT plugin access are unverified. Verify actual plugin availability and an authenticated `status` call after server/tunnel readiness; record the exact blocker if unavailable.
4. Only with separate paid-check approval, run a small supplied-evidence evaluation within strict limits, including retries in the cap. Record model identity, report ID/hash, citations, outcome and any gaps. A mobile `status` success alone does not prove evidence evaluation or semantic accuracy.

On shutdown or rollback, stop the current container/runtime completely before starting the replacement. Preserve separately versioned environment configuration: an image rollback does not restore earlier credentials or variable values. Outages and host/network failures can make tools unavailable; they are not missing-evidence or contradiction findings. Do not assume interrupted tool calls are replayed.

## Keep public HTTP as a separate alternative

The evidence HTTP adapter intentionally requires a loopback bind/peer and validates Host/Origin; forwarded headers do not grant trust. Do not change it to `0.0.0.0` or disable authentication merely to fit a CapRover web app. A static local bearer token is not public MCP OAuth.

A public route requires an implemented MCP-compatible OAuth 2.1 resource gateway, TLS termination and tested reverse proxy/auth discovery, resource/audience validation, denied origins, forwarding spoofing, cancellation and body/deadline limits. The gateway must validate external tokens, strip untrusted headers, rewrite the private Host and inject a separate internal token. The provider and tunnel keys remain private. That gateway is currently absent; [operations](operations.md) describes the readiness boundary. No public endpoint or tested CapRover deployment is claimed here.
