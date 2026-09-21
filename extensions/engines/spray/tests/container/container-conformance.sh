#!/bin/sh
set -eu

engine_path="${1:-/opt/lunafox-engine/bin/spray-engine}"
tools_dir="${2:-/opt/lunafox-tools/bin}"
versions_file="${3:-/opt/lunafox-tools/VERSIONS}"

fail() {
  echo "Spray Directory Fuzz runtime image conformance failed: $*" >&2
  exit 1
}

[ -r "$versions_file" ] || fail "missing version manifest"
[ -x "$engine_path" ] || fail "engine executable is missing or not executable at $engine_path"
[ -x "$tools_dir/spray" ] || fail "spray is not executable at $tools_dir/spray"
command -v spray >/dev/null 2>&1 || fail "spray is not on PATH"

marker="$(sed -n 's/^spray=//p' "$versions_file")"
[ "$marker" = "v0.3.2" ] || fail "spray marker is ${marker:-<missing>}, expected v0.3.2"

# Executability alone can be supplied by emulation. Match the ELF machine field
# to the selected container platform as independent architecture evidence.
machine="$(uname -m)"
elf_machine="$(od -An -tx1 -j 18 -N 2 "$tools_dir/spray" | tr -d ' \n')"
case "$machine:$elf_machine" in
  x86_64:3e00|aarch64:b700) ;;
  *) fail "spray architecture mismatch: runtime=$machine elf_machine=${elf_machine:-<missing>}" ;;
esac

if command -v docker >/dev/null 2>&1; then
  fail "runtime image must not include Docker CLI fallback"
fi

echo "Spray Directory Fuzz runtime image conformance passed"
