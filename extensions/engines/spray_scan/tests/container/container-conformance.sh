#!/bin/sh
set -eu
engine_path="${1:-/opt/lunafox-engine/bin/spray-scan-runtime-engine}"
test -x "$engine_path"
test -x /opt/lunafox-tools/bin/spray
test "$(cat /opt/lunafox-tools/VERSIONS)" = "spray=v0.3.2"
command -v spray >/dev/null 2>&1
if command -v docker >/dev/null 2>&1; then
  echo "runtime image contains Docker CLI" >&2
  exit 1
fi
echo "spray scan runtime image conformance passed"
