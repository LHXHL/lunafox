#!/bin/sh
set -eu
output=
previous=
set -- "$@"
: > /workspace/gogo.argv
for argument in "$@"; do
  if [ "$previous" = "-f" ]; then
    output="$argument"
  elif [ "$argument" != "-f" ]; then
    printf '%s\n' "$argument" >> /workspace/gogo.argv
  fi
  previous="$argument"
done
test -n "$output"
case "$output" in /workspace/gogo-*/results.json) ;; *) exit 1 ;; esac
printf '%s\n' '{"ip":"192.0.2.10","port":443,"protocol":"https"}' > "$output"
