#!/bin/sh
set -eu
engine_path="${1:-/opt/lunafox-engine/bin/zombie-audit-runtime-engine}"
test -x "$engine_path"
test -x /opt/lunafox-tools/bin/zombie
test "$(cat /opt/lunafox-tools/VERSIONS)" = "zombie=v1.3.0"
command -v zombie >/dev/null 2>&1
if command -v docker >/dev/null 2>&1; then
  echo "runtime image contains Docker CLI" >&2
  exit 1
fi
echo "zombie audit runtime image conformance passed"
