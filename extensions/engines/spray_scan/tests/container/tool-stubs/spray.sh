#!/bin/sh
set -eu
output=
previous=
: > /workspace/spray.argv
for argument in "$@"; do
  if [ "$previous" = "-f" ]; then
    output="$argument"
  elif [ "$argument" != "-f" ]; then
    printf '%s\n' "$argument" >> /workspace/spray.argv
  fi
  previous="$argument"
done
test -n "$output"
case "$output" in /workspace/spray-*/results.json) ;; *) exit 1 ;; esac
printf '%s\n' '{"url":"http://192.0.2.10/admin","status":403,"body_length":42,"spend":12,"content_type":"text/html"}' > "$output"
