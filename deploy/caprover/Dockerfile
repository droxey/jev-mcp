# Private CapRover stdio service for the evidence workflow.
# Build from the repository root, not from this directory.
FROM golang:1.27 AS evidence-build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" \
    -o /out/jev-evidence ./cmd/jev-evidence

FROM debian:12-slim AS tunnel-release

# v0.0.19 linux-amd64 ZIP; checksum verified against this release's official
# SHA256SUMS.txt, whose own SHA256 is recorded below. No credentials are args.
ARG TUNNEL_CLIENT_VERSION=v0.0.19
ARG TUNNEL_CLIENT_SHA256=9ee9448592345a9dd1517fb79791ab54d24f051142eb92bcbe8cd355effc9142
ARG TUNNEL_CLIENT_SUMS_SHA256=797c7a4e5790fad997739d5b520a7f4e5530d2d5886badfa1539142e3b048aac
ARG DEBIAN_FRONTEND=noninteractive
RUN apt-get update \
    && apt-get install --yes --no-install-recommends ca-certificates curl unzip \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /release
RUN set -eux; \
    base="https://github.com/openai/tunnel-client/releases/download/${TUNNEL_CLIENT_VERSION}"; \
    archive="tunnel-client-${TUNNEL_CLIENT_VERSION}-linux-amd64.zip"; \
    curl --fail --location --silent --show-error --retry 3 --output SHA256SUMS.txt "${base}/SHA256SUMS.txt"; \
    printf '%s  %s\n' "${TUNNEL_CLIENT_SUMS_SHA256}" SHA256SUMS.txt | sha256sum --check --strict; \
    curl --fail --location --silent --show-error --retry 3 --output "${archive}" "${base}/${archive}"; \
    printf '%s  %s\n' "${TUNNEL_CLIENT_SHA256}" "${archive}" | sha256sum --check --strict; \
    sums_line="$(awk -v name="${archive}" '$2 == name { print; count++ } END { if (count != 1) exit 1 }' SHA256SUMS.txt)"; \
    printf '%s\n' "${sums_line}" | sha256sum --check --strict; \
    unzip -q "${archive}" -d unpacked; \
    test -x unpacked/tunnel-client; \
    test -x unpacked/cloudflared; \
    unpacked/tunnel-client --version

# Debian slim supplies /bin/sh and dynamic glibc needed by the official client.
# Unlike the legacy bare-distroless image, it can execute the launcher. The final
# image has no release-download tools, source tree, or Go build tools.
FROM debian:12-slim AS runtime

RUN groupadd --gid 65532 nonroot \
    && useradd --uid 65532 --gid 65532 --no-create-home --home-dir /app nonroot \
    && mkdir -p /app/profile /app/runtime \
    && chown -R nonroot:nonroot /app/profile /app/runtime

COPY --from=evidence-build /out/jev-evidence /app/jev-evidence
COPY --from=tunnel-release /release/unpacked/tunnel-client /app/tunnel-client
COPY --from=tunnel-release /release/unpacked/cloudflared /app/cloudflared
COPY --from=tunnel-release /release/unpacked/cloudflared-manifest.json /app/cloudflared-manifest.json
COPY --from=tunnel-release /release/unpacked/LICENSE /app/LICENSE.tunnel-client
COPY --from=tunnel-release /release/unpacked/NOTICE /app/NOTICE.tunnel-client
COPY --from=tunnel-release /release/unpacked/tunnel-client-v0.0.19-linux-amd64-licenses.txt /app/tunnel-client-v0.0.19-linux-amd64-licenses.txt
COPY --from=tunnel-release /release/unpacked/tunnel-client-v0.0.19-linux-amd64.spdx.json /app/tunnel-client-v0.0.19-linux-amd64.spdx.json
COPY LICENSE /app/LICENSE.jev-mcp
COPY deploy/caprover/start-evidence /app/start-evidence
COPY deploy/caprover/start-tunnel /app/start-tunnel

ENV HOME=/app \
    XDG_CONFIG_HOME=/app/profile \
    XDG_CACHE_HOME=/app/runtime/cache \
    XDG_STATE_HOME=/app/runtime/state \
    TUNNEL_CLIENT_PROFILE_DIR=/app/profile/tunnel-client \
    TUNNEL_CLIENT_PROFILE=caprover-evidence \
    MCP_COMMAND=/app/start-evidence \
    MCP_STDIO_FAILURE_POLICY=exit \
    HEALTH_LISTEN_ADDR=127.0.0.1:8081

USER 65532:65532
WORKDIR /app
# Intentionally no EXPOSE or published port: the health listener is container-loopback only.
HEALTHCHECK --interval=30s --timeout=5s --start-period=45s --retries=3 \
    CMD ["/bin/sh", "-c", "exec /app/tunnel-client health --url \"http://${HEALTH_LISTEN_ADDR}\" --json --require-control-plane-poll"]
ENTRYPOINT ["/app/start-tunnel"]
