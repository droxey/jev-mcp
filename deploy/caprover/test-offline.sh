#!/bin/sh
# Repository checks that do not use a tunnel, provider credential, or Docker daemon.
set -eu

repo=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)
work=${TMPDIR:-/tmp}/jev-caprover-test.$$
trap 'rm -rf "$work"' EXIT HUP INT TERM
mkdir -p "$work/bin"
cd "$repo"

for name in TYPESAFE_API_KEY JEV_EVIDENCE_MODEL JEV_EVIDENCE_LIVE CONTROL_PLANE_API_KEY CONTROL_PLANE_TUNNEL_ID; do
    unset "$name"
done

shellcheck deploy/caprover/start-evidence deploy/caprover/start-tunnel deploy/caprover/test-offline.sh
go build -o "$work/bin/jev-evidence" ./cmd/jev-evidence
"$work/bin/jev-evidence" status > "$work/status.json"
"$work/bin/jev-evidence" lint --input testdata/evidence/plan.json > "$work/lint.json"
python3 - "$work/status.json" "$work/lint.json" <<'PY'
import json
import sys
status = json.load(open(sys.argv[1], encoding='utf-8'))
lint = json.load(open(sys.argv[2], encoding='utf-8'))
assert status['server'] == 'jev-evidence'
assert status['provider_configured'] is False
assert lint['valid'] is True
print('PASS offline status and fixture lint')
PY

# The launcher uses an absolute image path. Copy it to a scratch image layout
# and use bubblewrap when available; this test never contacts a control plane.
if command -v bwrap >/dev/null 2>&1; then
    mkdir -p "$work/app"
    cp deploy/caprover/start-evidence "$work/app/start-evidence"
    cp "$work/bin/jev-evidence" "$work/app/jev-evidence"
    python3 - "$work/app" <<'PY'
import json
import os
import pathlib
import select
import subprocess
import sys
app = sys.argv[1]
request = {'jsonrpc': '2.0', 'id': 1, 'method': 'initialize', 'params': {
    'protocolVersion': '2025-06-18', 'capabilities': {},
    'clientInfo': {'name': 'offline-packaging-test', 'version': '1'},
}}
p = subprocess.Popen([
    'bwrap', '--ro-bind', '/usr', '/usr', '--ro-bind', '/lib', '/lib',
    '--ro-bind', '/lib64', '/lib64', '--ro-bind', '/bin', '/bin',
    '--ro-bind', '/etc', '/etc', '--proc', '/proc', '--dev', '/dev',
    '--tmpfs', '/tmp', '--bind', app, '/app', '--', '/app/start-evidence',
], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
try:
    p.stdin.write(json.dumps(request) + '\n')
    p.stdin.flush()
    ready, _, _ = select.select([p.stdout], [], [], 5)
    assert ready, 'MCP initialize response timed out'
    line = p.stdout.readline()
    response = json.loads(line)
    assert response['id'] == 1
    assert response['result']['serverInfo']['name'] == 'jev-evidence'
    print('PASS fixed launcher stdio initialize; stdout contained one MCP frame')
finally:
    p.terminate()
    try:
        p.wait(timeout=5)
    except subprocess.TimeoutExpired:
        p.kill()
        p.wait(timeout=5)
PY
else
    printf '%s\n' 'SKIP container-path stdio launcher test: bwrap unavailable' >&2
fi
