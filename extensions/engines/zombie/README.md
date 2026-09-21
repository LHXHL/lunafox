# zombie engine (engine.lhxhl.zombie)

Third-party LunaFox engine wrapping [chainreactors/zombie](https://github.com/chainreactors/zombie)
v1.3.0 for weak-credential auditing. It is the first HostPorts fact consumer:
upstream `asset.host_port.v1` observations are mapped to zombie service
targets by a built-in port table, filtered by the `services` whitelist, and
audited in a single zombie run (targets carry `<service>://ip:port` scheme
URLs, the form zombie's ParseUrl accepts).

Successful logins are submitted as critical vulnerabilities:

- `vulnType`: `weak-credential:<service>`
- `severity`: `critical`
- `rawOutput`: `{ip, port, service, scheme, username, password}`

The canonical vulnerability URL schema only accepts http(s); the real service
scheme travels in `vulnType` and `rawOutput`.

## Boundary

- Server: target applicability, frozen manifest config, HostPorts fact input.
- Agent: mounts, workspace, runtime image lifecycle.
- Engine: port→service inference, whitelist filtering, zombie CLI invocation,
  JSONL parsing, typed submission.

## Notes

- This stage is intrusive. Keep it disabled in scan profiles and enable it
  only for explicitly authorized targets.
- `weakpass` enables zombie's weak-password rule generator; zombie requires a
  seed password for rule generation, so the engine passes `--pwd admin` as the
  seed.
- Ports not present in the built-in table (or filtered by the whitelist) are
  skipped and reported in the `input_ready` progress message.

## Contract regeneration

Regenerate `contract/` and `cmd/zombie-engine/execution_run_generated.go` from
`engine.json` with:

```sh
python3 scripts/engine-contract-gen.py extensions/engines/zombie
```

## Build

```sh
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --build-arg ENGINE_IMAGE_VERSION=0.0.0-dev \
  --build-context contracts=./contracts \
  --build-context engine-go=./engine-go \
  -f extensions/engines/zombie/Dockerfile \
  extensions/engines
```
