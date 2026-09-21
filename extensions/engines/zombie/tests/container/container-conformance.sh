#!/bin/sh
set -eu

engine_path="${1:-/opt/lunafox-engine/bin/zombie-engine}"
tools_dir="${2:-/opt/lunafox-tools/bin}"
versions_file="${3:-/opt/lunafox-tools/VERSIONS}"

fail() {
  echo "Zombie Credential Audit runtime image conformance failed: $*" >&2
  exit 1
}

[ -r "$versions_file" ] || fail "missing version manifest"
[ -x "$engine_path" ] || fail "engine executable is missing or not executable at $engine_path"
[ -x "$tools_dir/zombie" ] || fail "zombie is not executable at $tools_dir/zombie"
command -v zombie >/dev/null 2>&1 || fail "zombie is not on PATH"

marker="$(sed -n 's/^zombie=//p' "$versions_file")"
[ "$marker" = "v1.3.0" ] || fail "zombie marker is ${marker:-<missing>}, expected v1.3.0"

# Executability alone can be supplied by emulation. Match the ELF machine field
# to the selected container platform as independent architecture evidence.
machine="$(uname -m)"
elf_machine="$(od -An -tx1 -j 18 -N 2 "$tools_dir/zombie" | tr -d ' \n')"
case "$machine:$elf_machine" in
  x86_64:3e00|aarch64:b700) ;;
  *) fail "zombie architecture mismatch: runtime=$machine elf_machine=${elf_machine:-<missing>}" ;;
esac

if command -v docker >/dev/null 2>&1; then
  fail "runtime image must not include Docker CLI fallback"
fi

echo "Zombie Credential Audit runtime image conformance passed"
