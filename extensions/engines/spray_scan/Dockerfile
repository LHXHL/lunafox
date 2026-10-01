# syntax=docker/dockerfile:1.7

# lunafox:dockerfile-mode=standard
ARG ENGINE_GO_VERSION=1.26
ARG UBUNTU_BASE=public.ecr.aws/docker/library/ubuntu:noble-20260113
ARG ENGINE_IMAGE_VERSION
ARG ENGINE_IMAGE_SOURCE=https://github.com/yyhuni/lunafox
ARG GOPROXY=https://proxy.golang.org,direct

FROM --platform=$BUILDPLATFORM public.ecr.aws/docker/library/alpine:3.23.2 AS spray-downloader
ARG TARGETARCH
ARG SPRAY_VERSION=v0.3.2
WORKDIR /out
RUN set -eux; \
    case "$TARGETARCH" in \
      amd64) checksum="5e6c778790a93c79110394a1c43f8d1902e20c69f5f03e544347fda7cf9863e8" ;; \
      arm64) checksum="907185fc9cacae2b0040a96e183d0893201bb2f194d32fdd73eeb6851d7dd03d" ;; \
      *) echo "unsupported spray target architecture: $TARGETARCH" >&2; exit 1 ;; \
    esac; \
    attempt=1; \
    until wget -T 60 -O /out/spray "https://github.com/chainreactors/spray/releases/download/${SPRAY_VERSION}/spray_linux_${TARGETARCH}"; do \
      test "$attempt" -lt 5; attempt=$((attempt + 1)); rm -f /out/spray; sleep 2; \
    done; \
    printf '%s  %s\n' "$checksum" /out/spray | sha256sum -c -; \
    chmod 0755 /out/spray

FROM --platform=$BUILDPLATFORM public.ecr.aws/docker/library/golang:${ENGINE_GO_VERSION}-bookworm AS engine-builder
ARG TARGETOS=linux
ARG TARGETARCH
ARG GOPROXY
ENV CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GOTOOLCHAIN=local GOPROXY=$GOPROXY
COPY --from=contracts . /contracts
COPY --from=engine-go . /engine-go
WORKDIR /src
COPY go.mod go.sum ./
COPY chainreactor ./chainreactor
WORKDIR /src/spray_scan
COPY spray_scan/go.mod spray_scan/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    go mod download
COPY spray_scan ./
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    go build -trimpath -ldflags='-s -w' -o /out/spray-scan-runtime-engine ./cmd/spray-scan-engine

FROM --platform=$TARGETPLATFORM ${UBUNTU_BASE} AS runtime-base
ARG ENGINE_IMAGE_VERSION
ARG ENGINE_IMAGE_SOURCE
RUN test -n "$ENGINE_IMAGE_VERSION" && test -n "$ENGINE_IMAGE_SOURCE"
LABEL org.opencontainers.image.source="${ENGINE_IMAGE_SOURCE}" \
      org.opencontainers.image.version="${ENGINE_IMAGE_VERSION}"
ENV PATH=/opt/lunafox-tools/bin:$PATH
WORKDIR /workspace
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && install -d -m 0755 /opt/lunafox-engine/bin /opt/lunafox-tools/bin \
      /run/lunafox/context /run/lunafox/credential /run/lunafox/inputs \
      /run/lunafox/resources/config /run/lunafox/resources/platform \
      /run/lunafox/socket /workspace
COPY --from=spray-downloader /out/spray /opt/lunafox-tools/bin/spray
COPY --from=engine-builder /out/spray-scan-runtime-engine /opt/lunafox-engine/bin/spray-scan-runtime-engine
RUN chmod 0755 /opt/lunafox-tools/bin/spray /opt/lunafox-engine/bin/spray-scan-runtime-engine \
    && printf '%s\n' 'spray=v0.3.2' > /opt/lunafox-tools/VERSIONS

FROM runtime-base AS verify
RUN --mount=type=bind,source=spray_scan/tests/container,target=/run/lunafox-conformance \
    /bin/sh /run/lunafox-conformance/container-conformance.sh \
      /opt/lunafox-engine/bin/spray-scan-runtime-engine \
    && : > /conformance-passed

FROM runtime-base AS runtime
RUN --mount=type=bind,from=verify,source=/conformance-passed,target=/run/.conformance-passed \
    test -f /run/.conformance-passed
# Canonical task targets remain Agent-owned mounts: /run/lunafox/context/execution.pb,
# /run/lunafox/credential/token, /run/lunafox/socket/engine.sock,
# /run/lunafox/inputs/subdomains.txt, /run/lunafox/inputs/host-ports.jsonl,
# /run/lunafox/inputs/website-urls.txt, and /run/lunafox/inputs/endpoint-urls.txt.
USER 0:0
STOPSIGNAL SIGTERM
ENTRYPOINT ["/opt/lunafox-engine/bin/spray-scan-runtime-engine"]
CMD []
