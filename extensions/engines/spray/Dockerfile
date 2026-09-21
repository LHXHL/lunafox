# syntax=docker/dockerfile:1.7

# lunafox:dockerfile-mode=standard
# lunafox:generated:begin
ARG ENGINE_GO_VERSION=1.26
ARG UBUNTU_BASE=public.ecr.aws/docker/library/ubuntu:noble-20260113
ARG ENGINE_IMAGE_VERSION
ARG ENGINE_IMAGE_SOURCE=https://github.com/LHXHL/lunafox
ARG GOPROXY=https://proxy.golang.org,direct

# lunafox:engine-owned:build:begin
ARG SPRAY_VERSION=v0.3.2
FROM --platform=$BUILDPLATFORM public.ecr.aws/docker/library/alpine:3.23.2 AS spray-downloader
ARG TARGETARCH
ARG SPRAY_VERSION

WORKDIR /out
RUN set -eux; \
    case "$TARGETARCH" in \
      amd64) asset="spray_linux_amd64"; checksum="5e6c778790a93c79110394a1c43f8d1902e20c69f5f03e544347fda7cf9863e8" ;; \
      arm64) asset="spray_linux_arm64"; checksum="907185fc9cacae2b0040a96e183d0893201bb2f194d32fdd73eeb6851d7dd03d" ;; \
      *) echo "unsupported spray target architecture: $TARGETARCH" >&2; exit 1 ;; \
    esac; \
    url="https://github.com/chainreactors/spray/releases/download/${SPRAY_VERSION}/${asset}"; \
    attempt=1; \
    until wget -T 60 -O "/tmp/${asset}" "$url"; do \
      test "$attempt" -lt 5; \
      attempt=$((attempt + 1)); \
      rm -f "/tmp/${asset}"; \
      sleep 2; \
    done; \
    printf '%s  %s\n' "$checksum" "/tmp/${asset}" | sha256sum -c -; \
    install -m 0755 "/tmp/${asset}" /out/spray; \
    test -x /out/spray; \
    rm -f "/tmp/${asset}"
# lunafox:engine-owned:build:end

FROM --platform=$BUILDPLATFORM public.ecr.aws/docker/library/golang:${ENGINE_GO_VERSION}-bookworm AS engine-builder
ARG TARGETOS=linux
ARG TARGETARCH
ARG GOPROXY

ENV CGO_ENABLED=0 \
    GOOS=$TARGETOS \
    GOARCH=$TARGETARCH \
    GOTOOLCHAIN=local \
    GOPROXY=$GOPROXY

WORKDIR /src/spray
COPY --from=contracts . /contracts
COPY --from=engine-go . /engine-go
COPY spray/go.mod spray/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    go mod download
COPY spray ./
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    go build -trimpath -ldflags='-s -w' \
      -o /out/spray-engine \
      ./cmd/spray-engine

FROM --platform=$TARGETPLATFORM ${UBUNTU_BASE} AS runtime-base
ARG ENGINE_IMAGE_VERSION
ARG ENGINE_IMAGE_SOURCE

RUN test -n "$ENGINE_IMAGE_VERSION" && test -n "$ENGINE_IMAGE_SOURCE"

LABEL org.opencontainers.image.source="${ENGINE_IMAGE_SOURCE}" \
      org.opencontainers.image.version="${ENGINE_IMAGE_VERSION}"

ENV PATH=/opt/lunafox-tools/bin:$PATH

WORKDIR /workspace
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && install -d -m 0755 \
      /opt/lunafox-engine/bin \
      /opt/lunafox-tools/bin \
      /run/lunafox/context \
      /run/lunafox/credential \
      /run/lunafox/inputs \
      /run/lunafox/resources/config \
      /run/lunafox/resources/platform \
      /run/lunafox/socket \
      /workspace

# lunafox:engine-owned:runtime-base:begin
ARG SPRAY_VERSION
COPY --from=spray-downloader /out/spray /opt/lunafox-tools/bin/spray
RUN set -eux; \
    printf '%s\n' "spray=v0.3.2" > /opt/lunafox-tools/VERSIONS; \
    chmod 0755 /opt/lunafox-tools/bin/spray
# lunafox:engine-owned:runtime-base:end

COPY --from=engine-builder /out/spray-engine \
  /opt/lunafox-engine/bin/spray-engine
RUN chmod 0755 /opt/lunafox-engine/bin/spray-engine

FROM runtime-base AS verify
RUN --mount=type=bind,source=spray/tests/container,target=/run/lunafox-conformance \
    /bin/sh /run/lunafox-conformance/container-conformance.sh \
      /opt/lunafox-engine/bin/spray-engine \
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
ENTRYPOINT ["/opt/lunafox-engine/bin/spray-engine"]
CMD []
# lunafox:generated:end
