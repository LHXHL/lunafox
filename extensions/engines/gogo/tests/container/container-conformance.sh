#!/bin/sh
set -eu

engine_path="${1:-/opt/lunafox-engine/bin/gogo-engine}"
tools_dir="${2:-/opt/lunafox-tools/bin}"
versions_file="${3:-/opt/lunafox-tools/VERSIONS}"

fail() {
  echo "Gogo Recon runtime image conformance failed: $*" >&2
  exit 1
}

[ -r "$versions_file" ] || fail "missing version manifest"
[ -x "$engine_path" ] || fail "engine executable is missing or not executable at $engine_path"
[ -x "$tools_dir/gogo" ] || fail "gogo is not executable at $tools_dir/gogo"
command -v gogo >/dev/null 2>&1 || fail "gogo is not on PATH"

marker="$(sed -n 's/^gogo=//p' "$versions_file")"
[ "$marker" = "v2.15.0" ] || fail "gogo marker is ${marker:-<missing>}, expected v2.15.0"

# Executability alone can be supplied by emulation. Match the ELF machine field
# to the selected container platform as independent architecture evidence.
machine="$(uname -m)"
elf_machine="$(od -An -tx1 -j 18 -N 2 "$tools_dir/gogo" | tr -d ' \n')"
case "$machine:$elf_machine" in
  x86_64:3e00|aarch64:b700) ;;
  *) fail "gogo architecture mismatch: runtime=$machine elf_machine=${elf_machine:-<missing>}" ;;
esac

if command -v docker >/dev/null 2>&1; then
  fail "runtime image must not include Docker CLI fallback"
fi

echo "Gogo Recon runtime image conformance passed"
