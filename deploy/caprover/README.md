# Private CapRover evidence service

This image runs `jev-evidence` over stdio beneath the official OpenAI tunnel client. It does not run the legacy `cmd/jev-mcp` server and does not create a public HTTP MCP endpoint. The upstream `tphakala/jev-mcp` Apache-2.0 notice at `f51a9267f74a641f44f90fa5e1a8d3125edc9a35` is preserved in `/app/LICENSE.jev-mcp`.

## Build and verify

Build from the repository root with network access to Go modules, Debian packages, and the official GitHub release:

```sh
docker build --file deploy/caprover/Dockerfile --tag jev-evidence-caprover:local .
docker image inspect jev-evidence-caprover:local --format '{{.Id}} {{.Config.User}} {{json .Config.Entrypoint}}'
docker history --no-trunc jev-evidence-caprover:local
docker run --rm --entrypoint /app/jev-evidence jev-evidence-caprover:local status
docker run --rm --entrypoint /app/tunnel-client jev-evidence-caprover:local --version
```

The runtime is `debian:12-slim`, UID/GID `65532`, with `/bin/sh` and glibc for the two `exec` launchers and the official linux-amd64 client. It is intentionally not bare distroless. It retains CA roots and has writable, nonsecret `/app/profile` and `/app/runtime` directories. The full pinned ZIP executable/companion set, manifest, LICENSE, NOTICE, licenses, and SPDX inventory stay together under `/app`; the default path does not enable Cloudflare.

Pinned release (verified 2026-11-05):

| Asset | SHA-256 |
| --- | --- |
| `tunnel-client-v0.0.19-linux-amd64.zip` | `9ee9448592345a9dd1517fb79791ab54d24f051142eb92bcbe8cd355effc9142` |
| `SHA256SUMS.txt` | `797c7a4e5790fad997739d5b520a7f4e5530d2d5886badfa1539142e3b048aac` |

The build verifies the checksums file against its pinned digest, verifies the ZIP against both the pinned digest and that file, then executes `tunnel-client --version`. It also keeps the matching release licenses and SPDX JSON. The release has a provenance attestation, but this build verifies SHA-256 only; it does not claim a Sigstore verification. This package supports **linux/amd64** because the selected asset and Go output are both fixed to that architecture.

`start-tunnel` rejects non-loopback health addresses and unexpected stdio/failure-policy overrides, then `exec`s `tunnel-client run --profile caprover-evidence`. `start-evidence` uses `exec` for this fixed command:

```sh
/app/jev-evidence serve --transport stdio --timeout 90s --max-calls 16 --batch-size 4 --concurrency 1 --retries 0
```

The pinned client directly starts, signals, and waits for the evidence child. `MCP_STDIO_FAILURE_POLICY=exit` is supported in v0.0.19: unexpected child loss exits the client for CapRover to restart; intentional shutdown remains clean. Evidence stdout is MCP only, and diagnostics go to stderr. The Docker healthcheck calls the pinned client's `health --json --require-control-plane-poll` against `${HEALTH_LISTEN_ADDR}`.

## Local verification status

On 2026-11-05 the release checksum, binary version/ELF architecture, fixed stdio command, MCP initialize stream, non-loopback rejection, failed-child exit, and TERM cleanup were tested offline with the pinned v0.0.19 binary and an unreachable loopback control-plane endpoint. The Docker daemon was unavailable (`unix:///var/run/docker.sock: no such file or directory`), so the image build, image digest, nonroot inspection, and layer-history checks remain required on a Docker-enabled host before Production handoff. No paid provider call or live tunnel was used.

## Runtime configuration

Create the nonsecret profile in CapRover's persistent app data, or generate it during an authorized Production setup with the official command. Do not run `init` as a build step or against a new/unapproved tunnel association. The profile must select `/app/start-evidence`; the image's fixed `MCP_COMMAND` provides the same binding when the profile leaves it unset.

```sh
tunnel-client init --sample sample_mcp_stdio_local \
  --profile caprover-evidence --tunnel-id <approved-tunnel-id> \
  --mcp-command /app/start-evidence
```

Place the resulting file at `/app/profile/tunnel-client/caprover-evidence.yaml`, with `control_plane.api_key: env:CONTROL_PLANE_API_KEY`, loopback health, and `admin_ui.open_browser: false`. The generated profile may also contain the reviewed tunnel ID; it must not contain literal keys. A persistent app-data mount at `/app/profile` is for this nonsecret tunnel profile only; no evidence database or volume is needed. `/app/runtime` is container-local ephemeral state. Inspect the profile before deployment: it must contain only environment/file secret references, never literal credentials.

| Variable or setting | Required handling |
| --- | --- |
| `TYPESAFE_API_KEY` | Required for paid evidence evaluation; inject through CapRover runtime secrets. Never put it in the image, profile, MCP arguments, build args, logs, or repository. Offline `status`/`lint` work without it. |
| `JEV_EVIDENCE_MODEL` | Explicit supported model ID, preferably immutable. Required with the provider key for evaluation; no silent model fallback. |
| `CONTROL_PLANE_API_KEY` | Required restricted tunnel **runtime** key. Never use an admin key for the daemon. Inject as a runtime secret and reference it with `env:CONTROL_PLANE_API_KEY`. |
| `CONTROL_PLANE_TUNNEL_ID` | Existing approved tunnel ID when not recorded in the nonsecret profile. Do not create, associate, expand, or repoint tunnels as packaging work. |
| `TUNNEL_CLIENT_PROFILE` | Default `caprover-evidence`; selects the nonsecret profile in `/app/profile/tunnel-client`. |
| `MCP_COMMAND` | Fixed to `/app/start-evidence`; the launcher rejects substitutions. |
| `MCP_STDIO_FAILURE_POLICY` | Fixed to `exit`, supported by v0.0.19, so the external supervisor owns whole-runtime restart. |
| `HEALTH_LISTEN_ADDR` | Default `127.0.0.1:8081`. Only IPv4/IPv6 loopback is accepted. Do not map this port or expose the admin UI. |

No secret is required merely to build the image. CapRover runtime variables and backups may contain secrets: restrict their access. Image rollback does not roll back environment values.

## CapRover settings and handoff

Suggested app name: **`jev-evidence-private`** (create it only under an authorized Production handoff).

1. Use the root `captain-definition` source archive. Select **Do not expose as web app**, publish **no ports**, and set exactly **one instance**. The Dockerfile intentionally declares no `EXPOSE` port.
2. Paste `service-update-override.yml` into **Service Update Override**. It sets `UpdateConfig` and `RollbackConfig` to `Parallelism: 1`, `Order: stop-first`. This intentionally causes a brief outage and prevents two clients sharing the stdio tunnel during rollout or rollback.
3. Mount the reviewed nonsecret profile and inject the runtime secrets above. Do not run a second local/client instance against that tunnel while Production is active.
4. Before activation, verify one task, stop-first service settings, no public route/ports, and loopback-only health. After activation, use `tunnel-client health --url http://127.0.0.1:8081 --json` and `doctor --profile caprover-evidence --explain` inside the container. Check `/health/mcp` after a real request; stdio readiness alone does not prove tool discovery or a cloud/mobile round trip.

No deployment, provider call, tunnel initialization, or authenticated ChatGPT/iPhone test is performed by this packaging change.
