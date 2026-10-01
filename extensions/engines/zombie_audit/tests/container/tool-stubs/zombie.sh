#!/bin/sh
set -eu
output=
targets=
previous=
: > /workspace/zombie.argv
for argument in "$@"; do
  case "$previous" in
    -f) output="$argument" ;;
    -I) targets="$argument" ;;
  esac
  case "$argument" in
    -f|-I) printf '%s\n' "$argument" >> /workspace/zombie.argv ;;
    *) if [ "$previous" != "-f" ] && [ "$previous" != "-I" ]; then printf '%s\n' "$argument" >> /workspace/zombie.argv; fi ;;
  esac
  previous="$argument"
done
test -n "$output"
test -n "$targets"
test -f "$targets"
test "$(cat "$targets")" = "http://192.0.2.10:80"
case "$output" in /workspace/zombie-*/results.json) ;; *) exit 1 ;; esac
printf '%s\n' '{"ip":"192.0.2.10","port":80,"service":"http","username":"fixture","password":"fixture","mod":0}' > "$output"
