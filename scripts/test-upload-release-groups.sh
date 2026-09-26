#!/bin/bash
# Regression test for scripts/upload-release.sh target groups, against a local
# stub server (never a real updates server).
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
script="$here/upload-release.sh"
work="$(mktemp -d)"
stub_pid=""
cleanup() {
    if [ -n "$stub_pid" ]; then kill "$stub_pid" 2>/dev/null || true; fi
    rm -rf "$work"
}
trap cleanup EXIT

bash -n "$script"

if grep -Eq '(^|[^[:alnum:]_])GROUPS[[:space:]]*=' "$script"; then
    echo "FAIL: upload-release.sh assigns the Bash builtin GROUPS" >&2
    exit 1
fi

port=$((20000 + RANDOM % 20000))
python3 - "$port" "$work/requests.log" >"$work/stub.out" 2>&1 <<'PY' &
import sys, http.server
port, log = int(sys.argv[1]), sys.argv[2]
class H(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        body = self.rfile.read(int(self.headers.get("Content-Length", 0)))
        with open(log, "ab") as f:
            f.write(body + b"\n--END--\n")
        self.send_response(201)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(b'{"id":"stub"}')
    def log_message(self, *a):
        pass
http.server.HTTPServer(("127.0.0.1", port), H).serve_forever()
PY
stub_pid=$!
disown "$stub_pid"
for _ in $(seq 1 50); do
    curl -s -o /dev/null "http://127.0.0.1:$port/" && break
    sleep 0.1
done

printf 'artifact' > "$work/artifact.tar.gz"
run() {
    printf 'y\n' | bash "$script" --product siemcore --version 9.9.9.1 \
        --file "$work/artifact.tar.gz" --server "http://127.0.0.1:$port" \
        --api-key test-key "$@"
}
requests() { grep -c -- '--END--' "$work/requests.log" 2>/dev/null || echo 0; }

if run >"$work/out1" 2>&1; then
    echo "FAIL: upload without --groups succeeded" >&2; exit 1
fi
grep -q -- '--groups is required' "$work/out1"
[ "$(requests)" = 0 ] || { echo "FAIL: request sent without --groups" >&2; exit 1; }

if run --groups alpha,prod >"$work/out2" 2>&1; then
    echo "FAIL: invalid group accepted" >&2; exit 1
fi
grep -q "invalid target group 'prod'" "$work/out2"
[ "$(requests)" = 0 ] || { echo "FAIL: request sent with invalid group" >&2; exit 1; }

run --groups alpha >"$work/out3" 2>&1
[ "$(requests)" = 1 ] || { echo "FAIL: expected one upload request" >&2; exit 1; }
tr -d '\r' < "$work/requests.log" | grep -A2 'name="target_groups"' | grep -qx 'alpha' || {
    echo "FAIL: target_groups=alpha not sent" >&2; exit 1; }

echo "upload-release target groups: required, validated, alpha passed through"
