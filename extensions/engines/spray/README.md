# spray engine (engine.lhxhl.spray)

Third-party LunaFox engine wrapping [chainreactors/spray](https://github.com/chainreactors/spray)
v0.3.2 as a replacement for the first-party ffuf directory scan. It consumes
the WebsiteURLs fact input (same input contract as `engine.lunafox.directory_scan`)
and submits:

| spray output | canonical result |
| --- | --- |
| valid/fuzzy path observation | `asset.directory.v1` |
| frameworks | `asset.website_technology.v1` |

## Boundary

- Server: target applicability, frozen manifest config, wordlist resource
  binding, WebsiteURLs fact input.
- Agent: mounts, workspace, runtime image lifecycle.
- Engine: candidate validation and dedup, spray CLI invocation, JSONL
  parsing, typed submission.

## Notes

- Results are read from the `--file-output json` artifact (one
  `parsers.SprayResult` per line); stdout is drained.
- The wordlist arrives as a platform resource binding at
  `/run/lunafox/resources/config/spray/wordlist/...` and is passed via
  `--dict`.

## Contract regeneration

Regenerate `contract/` and `cmd/spray-engine/execution_run_generated.go` from
`engine.json` with:

```sh
python3 scripts/engine-contract-gen.py extensions/engines/spray
```

## Build

```sh
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --build-arg ENGINE_IMAGE_VERSION=0.0.0-dev \
  --build-context contracts=./contracts \
  --build-context engine-go=./engine-go \
  -f extensions/engines/spray/Dockerfile \
  extensions/engines
```
