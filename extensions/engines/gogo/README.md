# gogo engine (engine.lhxhl.gogo)

Third-party LunaFox engine wrapping [chainreactors/gogo](https://github.com/chainreactors/gogo)
v2.15.0. One execution performs port scanning, service probing, web
fingerprinting, and optional neutron POC detection, and submits four canonical
result streams:

| gogo output | canonical result |
| --- | --- |
| open port line | `asset.host_port.v1` |
| http/https service | `asset.website.v1` |
| frameworks | `asset.website_technology.v1` |
| vulns (neutron POC hits) | `asset.vulnerability.v1` |

## Boundary

- Server: target applicability, frozen manifest config, subdomains fact input.
- Agent: mounts, workspace, runtime image lifecycle.
- Engine: candidate preparation (IPv4 resolution for domain targets, CIDR
  passthrough), gogo CLI invocation, JSONL parsing, typed submission.

## Notes

- gogo accepts only IP/CIDR targets; domain targets (and the Subdomains fact
  input) are resolved to IPv4 by the engine before scanning.
- `asset.vulnerability.v1` URLs must be http(s); vulns found on non-web
  protocols keep the real protocol in `rawOutput` and use an `http://ip:port`
  locator.
- Output is forced to plain JSONL via `--file-output jl --compress` (the
  `--compress` flag disables deflate wrapping).

## Contract regeneration

`contract/` and `cmd/gogo-engine/execution_run_generated.go` are produced from
`engine.json` by the upstream `engine-manifest-artifact-gen` tool, which is not
exported. Regenerate with the minimal splice tool shipped in this fork:

```sh
python3 scripts/engine-contract-gen.py extensions/engines/gogo
```

`engine.json` remains the single source of truth; keep the generated Config
block in sync when params change.

## Build

```sh
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --build-arg ENGINE_IMAGE_VERSION=0.0.0-dev \
  --build-context contracts=./contracts \
  --build-context engine-go=./engine-go \
  -f extensions/engines/gogo/Dockerfile \
  extensions/engines
```
