#!/usr/bin/env python3
"""Emit the remaining boilerplate for the chainreactors engines:
Dockerfile, tests/container artifacts, locales (en/zh). Run from repo root."""
import json
import os

ENGINES = {
    "gogo": {
        "tool": "gogo",
        "version": "v2.15.0",
        "repo": "chainreactors/gogo",
        "checksums": {
            "amd64": "3b51080a909023f7d484e2443740b572307ce0054eb9e32d98f0a06bfcd2db90",
            "arm64": "f34e248bc1d0f8e18101655850082b9f48f5f9f5a102cbac107086f3798c20a9",
        },
        "display": "Gogo Recon",
        "description_en": "Port scan, service probe, fingerprint, and POC detection in one gogo run.",
        "description_zh": "一次 gogo 执行完成端口扫描、服务探测、指纹识别与 POC 检测。",
        "section": "gogo",
        "section_display": "Gogo",
        "section_desc_en": "Configure the gogo recon run.",
        "section_desc_zh": "配置 gogo 侦察执行。",
        "params": {
            "port-tag": ("Port preset tag passed to gogo -p (top1/top2/top3/common/all...).",
                         "传递给 gogo -p 的端口预设标签（top1/top2/top3/common/all 等）。"),
            "ports": ("Explicit port expression that overrides port-tag when set.",
                      "显式端口表达式，设置后覆盖 port-tag。"),
            "concurrency": ("Concurrent thread count passed to gogo -t.",
                            "传递给 gogo -t 的并发线程数。"),
            "timeout": ("Socket and HTTP timeout in seconds passed to gogo -d.",
                        "传递给 gogo -d 的 socket 与 HTTP 超时（秒）。"),
            "mod": ("gogo smart mode: default, s, ss, or sc.",
                    "gogo 智能模式：default、s、ss 或 sc。"),
            "exploit": ("Enable gogo neutron POC vulnerability detection.",
                        "启用 gogo neutron POC 漏洞检测。"),
            "active-finger": ("Enable gogo active fingerprint scanning (-v).",
                              "启用 gogo 主动指纹扫描（-v）。"),
        },
    },
    "spray": {
        "tool": "spray",
        "version": "v0.3.2",
        "repo": "chainreactors/spray",
        "checksums": {
            "amd64": "5e6c778790a93c79110394a1c43f8d1902e20c69f5f03e544347fda7cf9863e8",
            "arm64": "907185fc9cacae2b0040a96e183d0893201bb2f194d32fdd73eeb6851d7dd03d",
        },
        "display": "Spray Directory Fuzz",
        "description_en": "Directory and path fuzzing with spray, including smart filtering and fingerprints.",
        "description_zh": "使用 spray 进行目录与路径爆破，含智能过滤与指纹识别。",
        "section": "spray",
        "section_display": "Spray",
        "section_desc_en": "Configure the spray directory fuzzing run.",
        "section_desc_zh": "配置 spray 目录爆破执行。",
        "params": {
            "wordlist": ("Wordlist resource binding used as the spray dictionary.",
                         "作为 spray 字典的字典资源绑定。"),
            "pool": ("Concurrent pool count passed to spray --pool.",
                     "传递给 spray --pool 的并发池数量。"),
            "threads": ("Threads per pool passed to spray --thread.",
                        "传递给 spray --thread 的每池线程数。"),
            "request-timeout": ("Per-request timeout in seconds passed to spray --timeout.",
                                "传递给 spray --timeout 的单请求超时（秒）。"),
            "mod": ("spray fuzz mode: path or host.",
                    "spray 爆破模式：path 或 host。"),
        },
    },
    "zombie": {
        "tool": "zombie",
        "version": "v1.3.0",
        "repo": "chainreactors/zombie",
        "checksums": {
            "amd64": "94cc277b812d5b056984fb2206d32d9911791feea7a0dc811c2b196a82666325",
            "arm64": "0bb305893e0639cd74fe4918782f8724e96a0ef97bba590f10f7c9ff0f4fc5c7",
        },
        "display": "Zombie Credential Audit",
        "description_en": "Weak-credential auditing for discovered services with zombie; findings are reported as critical vulnerabilities.",
        "description_zh": "使用 zombie 对已发现服务进行弱口令审计，命中结果按严重漏洞上报。",
        "section": "zombie",
        "section_display": "Zombie",
        "section_desc_en": "Configure the zombie credential audit run.",
        "section_desc_zh": "配置 zombie 口令审计执行。",
        "params": {
            "services": ("Whitelist of zombie service plugins eligible for auditing.",
                         "允许审计的 zombie 服务插件白名单。"),
            "weakpass": ("Generate password candidates from weak-password rules (seed required).",
                         "启用弱口令规则生成密码候选（需要种子密码）。"),
            "threads": ("Concurrent thread count passed to zombie --thread.",
                        "传递给 zombie --thread 的并发线程数。"),
        },
    },
}

DOCKERFILE_TEMPLATE = """# syntax=docker/dockerfile:1.7

# lunafox:dockerfile-mode=standard
# lunafox:generated:begin
ARG ENGINE_GO_VERSION=1.26
ARG UBUNTU_BASE=public.ecr.aws/docker/library/ubuntu:noble-20260113
ARG ENGINE_IMAGE_VERSION
ARG ENGINE_IMAGE_SOURCE=https://github.com/LHXHL/lunafox
ARG GOPROXY=https://proxy.golang.org,direct

# lunafox:engine-owned:build:begin
ARG {TOOL_UPPER}_VERSION={VERSION}
FROM --platform=$BUILDPLATFORM public.ecr.aws/docker/library/alpine:3.23.2 AS {TOOL}-downloader
ARG TARGETARCH
ARG {TOOL_UPPER}_VERSION

WORKDIR /out
RUN set -eux; \\
    case "$TARGETARCH" in \\
      amd64) asset="{TOOL}_linux_amd64"; checksum="{SHA_AMD64}" ;; \\
      arm64) asset="{TOOL}_linux_arm64"; checksum="{SHA_ARM64}" ;; \\
      *) echo "unsupported {TOOL} target architecture: $TARGETARCH" >&2; exit 1 ;; \\
    esac; \\
    url="https://github.com/{REPO}/releases/download/${{{TOOL_UPPER}_VERSION}}/${{asset}}"; \\
    attempt=1; \\
    until wget -T 60 -O "/tmp/${{asset}}" "$url"; do \\
      test "$attempt" -lt 5; \\
      attempt=$((attempt + 1)); \\
      rm -f "/tmp/${{asset}}"; \\
      sleep 2; \\
    done; \\
    printf '%s  %s\\n' "$checksum" "/tmp/${{asset}}" | sha256sum -c -; \\
    install -m 0755 "/tmp/${{asset}}" /out/{TOOL}; \\
    test -x /out/{TOOL}; \\
    rm -f "/tmp/${{asset}}"
# lunafox:engine-owned:build:end

FROM --platform=$BUILDPLATFORM public.ecr.aws/docker/library/golang:${{ENGINE_GO_VERSION}}-bookworm AS engine-builder
ARG TARGETOS=linux
ARG TARGETARCH
ARG GOPROXY

ENV CGO_ENABLED=0 \\
    GOOS=$TARGETOS \\
    GOARCH=$TARGETARCH \\
    GOTOOLCHAIN=local \\
    GOPROXY=$GOPROXY

WORKDIR /src/{NAME}
COPY --from=contracts . /contracts
COPY --from=engine-go . /engine-go
COPY {NAME}/go.mod {NAME}/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \\
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \\
    go mod download
COPY {NAME} ./
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \\
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \\
    go build -trimpath -ldflags='-s -w' \\
      -o /out/{NAME}-engine \\
      ./cmd/{NAME}-engine

FROM --platform=$TARGETPLATFORM ${{UBUNTU_BASE}} AS runtime-base
ARG ENGINE_IMAGE_VERSION
ARG ENGINE_IMAGE_SOURCE

RUN test -n "$ENGINE_IMAGE_VERSION" && test -n "$ENGINE_IMAGE_SOURCE"

LABEL org.opencontainers.image.source="${{ENGINE_IMAGE_SOURCE}}" \\
      org.opencontainers.image.version="${{ENGINE_IMAGE_VERSION}}"

ENV PATH=/opt/lunafox-tools/bin:$PATH

WORKDIR /workspace
RUN apt-get update && apt-get install -y --no-install-recommends \\
      ca-certificates \\
    && rm -rf /var/lib/apt/lists/* \\
    && install -d -m 0755 \\
      /opt/lunafox-engine/bin \\
      /opt/lunafox-tools/bin \\
      /run/lunafox/context \\
      /run/lunafox/credential \\
      /run/lunafox/inputs \\
      /run/lunafox/resources/config \\
      /run/lunafox/resources/platform \\
      /run/lunafox/socket \\
      /workspace

# lunafox:engine-owned:runtime-base:begin
ARG {TOOL_UPPER}_VERSION
COPY --from={TOOL}-downloader /out/{TOOL} /opt/lunafox-tools/bin/{TOOL}
RUN set -eux; \\
    printf '%s\\n' "{TOOL}={VERSION}" > /opt/lunafox-tools/VERSIONS; \\
    chmod 0755 /opt/lunafox-tools/bin/{TOOL}
# lunafox:engine-owned:runtime-base:end

COPY --from=engine-builder /out/{NAME}-engine \\
  /opt/lunafox-engine/bin/{NAME}-engine
RUN chmod 0755 /opt/lunafox-engine/bin/{NAME}-engine

FROM runtime-base AS verify
RUN --mount=type=bind,source={NAME}/tests/container,target=/run/lunafox-conformance \\
    /bin/sh /run/lunafox-conformance/container-conformance.sh \\
      /opt/lunafox-engine/bin/{NAME}-engine \\
    && : > /conformance-passed

FROM runtime-base AS runtime
RUN --mount=type=bind,from=verify,source=/conformance-passed,target=/run/.conformance-passed \\
    test -f /run/.conformance-passed

# Canonical task targets remain Agent-owned mounts: /run/lunafox/context/execution.pb,
# /run/lunafox/credential/token, /run/lunafox/socket/engine.sock,
# /run/lunafox/inputs/subdomains.txt, /run/lunafox/inputs/host-ports.jsonl,
# /run/lunafox/inputs/website-urls.txt, and /run/lunafox/inputs/endpoint-urls.txt.
USER 0:0
STOPSIGNAL SIGTERM
ENTRYPOINT ["/opt/lunafox-engine/bin/{NAME}-engine"]
CMD []
# lunafox:generated:end
"""

CONFORMANCE_TEMPLATE = """#!/bin/sh
set -eu

engine_path="${{1:-/opt/lunafox-engine/bin/{NAME}-engine}}"
tools_dir="${{2:-/opt/lunafox-tools/bin}}"
versions_file="${{3:-/opt/lunafox-tools/VERSIONS}}"

fail() {{
  echo "{DISPLAY} runtime image conformance failed: $*" >&2
  exit 1
}}

[ -r "$versions_file" ] || fail "missing version manifest"
[ -x "$engine_path" ] || fail "engine executable is missing or not executable at $engine_path"
[ -x "$tools_dir/{TOOL}" ] || fail "{TOOL} is not executable at $tools_dir/{TOOL}"
command -v {TOOL} >/dev/null 2>&1 || fail "{TOOL} is not on PATH"

marker="$(sed -n 's/^{TOOL}=//p' "$versions_file")"
[ "$marker" = "{VERSION}" ] || fail "{TOOL} marker is ${{marker:-<missing>}}, expected {VERSION}"

# Executability alone can be supplied by emulation. Match the ELF machine field
# to the selected container platform as independent architecture evidence.
machine="$(uname -m)"
elf_machine="$(od -An -tx1 -j 18 -N 2 "$tools_dir/{TOOL}" | tr -d ' \\n')"
case "$machine:$elf_machine" in
  x86_64:3e00|aarch64:b700) ;;
  *) fail "{TOOL} architecture mismatch: runtime=$machine elf_machine=${{elf_machine:-<missing>}}" ;;
esac

if command -v docker >/dev/null 2>&1; then
  fail "runtime image must not include Docker CLI fallback"
fi

echo "{DISPLAY} runtime image conformance passed"
"""


def main():
    base = os.path.join("extensions", "engines")
    for name, spec in ENGINES.items():
        root = os.path.join(base, name)
        tool = spec["tool"]
        substitutions = {
            "NAME": name,
            "TOOL": tool,
            "TOOL_UPPER": tool.upper(),
            "VERSION": spec["version"],
            "REPO": spec["repo"],
            "SHA_AMD64": spec["checksums"]["amd64"],
            "SHA_ARM64": spec["checksums"]["arm64"],
            "DISPLAY": spec["display"],
        }
        dockerfile = DOCKERFILE_TEMPLATE
        conformance = CONFORMANCE_TEMPLATE
        for key, value in substitutions.items():
            dockerfile = dockerfile.replace("{" + key + "}", value)
            conformance = conformance.replace("{" + key + "}", value)
        # Collapse the format-style brace escapes now that placeholders are gone.
        dockerfile = dockerfile.replace("{{", "{").replace("}}", "}")
        conformance = conformance.replace("{{", "{").replace("}}", "}")

        with open(os.path.join(root, "Dockerfile"), "w", encoding="utf-8") as handle:
            handle.write(dockerfile)

        tests_dir = os.path.join(root, "tests", "container")
        os.makedirs(tests_dir, exist_ok=True)
        with open(os.path.join(tests_dir, "container-conformance.sh"), "w", encoding="utf-8") as handle:
            handle.write(conformance)

        conformance_json = {
            "schemaVersion": "lunafox-engine-image-conformance-profile/v1",
            "target": {"type": "ip", "value": "192.0.2.10"},
            "noOpEnabledSections": [spec["section"]],
            "toolEnabledSections": [spec["section"]],
            "gateDefaultProgressAck": False,
            "resultType": "asset.vulnerability.v1" if name == "zombie" else "asset.host_port.v1",
            "toolStubs": [],
            "argvAssertions": [],
            "platformResources": [],
            "imageLocalProbes": [],
        }
        if name == "spray":
            conformance_json["target"] = {"type": "domain", "value": "example.com"}
            conformance_json["resultType"] = "asset.directory.v1"
        with open(os.path.join(tests_dir, "image-conformance.json"), "w", encoding="utf-8") as handle:
            json.dump(conformance_json, handle, indent=2)
            handle.write("\n")

        for lang, desc_index, name_key in (("en", 0, "displayName"), ("zh", 1, "displayName")):
            locale = {
                "engine": {
                    "displayName": spec["display"],
                    "description": spec["description_en"] if lang == "en" else spec["description_zh"],
                },
                "sections": {
                    spec["section"]: {
                        "name": spec["section_display"],
                        "description": spec["section_desc_en"] if lang == "en" else spec["section_desc_zh"],
                        "params": {
                            key: {"description": descriptions[desc_index]}
                            for key, descriptions in spec["params"].items()
                        },
                    }
                },
            }
            with open(os.path.join(root, "locales", f"{lang}.json"), "w", encoding="utf-8") as handle:
                json.dump(locale, handle, ensure_ascii=False, indent=2)
                handle.write("\n")
        print(f"emitted boilerplate for {name}")


if __name__ == "__main__":
    main()
