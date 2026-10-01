#!/bin/sh
set -eu

engine_path="${1:-/opt/lunafox-engine/bin/gogo-scan-runtime-engine}"
test -x "$engine_path"
test -x /opt/lunafox-tools/bin/gogo
test "$(cat /opt/lunafox-tools/VERSIONS)" = "gogo=v2.15.0"
command -v gogo >/dev/null 2>&1
if command -v docker >/dev/null 2>&1; then
  echo "runtime image contains Docker CLI" >&2
  exit 1
fi
echo "gogo scan runtime image conformance passed"
