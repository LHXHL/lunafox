# syntax=docker/dockerfile:1.7

# lunafox:dockerfile-mode=standard
# lunafox:generated:begin
ARG ENGINE_GO_VERSION=1.26
ARG UBUNTU_BASE=public.ecr.aws/docker/library/ubuntu:noble-20260113
ARG ENGINE_IMAGE_VERSION
ARG ENGINE_IMAGE_SOURCE=https://github.com/LHXHL/lunafox
ARG GOPROXY=https://proxy.golang.org,direct

# lunafox:engine-owned:build:begin
ARG ZOMBIE_VERSION=v1.3.0
FROM --platform=$BUILDPLATFORM public.ecr.aws/docker/library/alpine:3.23.2 AS zombie-downloader
ARG TARGETARCH
ARG ZOMBIE_VERSION

WORKDIR /out
RUN set -eux; \
    case "$TARGETARCH" in \
      amd64) asset="zombie_linux_amd64"; checksum="94cc277b812d5b056984fb2206d32d9911791feea7a0dc811c2b196a82666325" ;; \
      arm64) asset="zombie_linux_arm64"; checksum="0bb305893e0639cd74fe4918782f8724e96a0ef97bba590f10f7c9ff0f4fc5c7" ;; \
      *) echo "unsupported zombie target architecture: $TARGETARCH" >&2; exit 1 ;; \
    esac; \
    url="https://github.com/chainreactors/zombie/releases/download/${ZOMBIE_VERSION}/${asset}"; \
    attempt=1; \
    until wget -T 60 -O "/tmp/${asset}" "$url"; do \
      test "$attempt" -lt 5; \
      attempt=$((attempt + 1)); \
      rm -f "/tmp/${asset}"; \
      sleep 2; \
    done; \
    printf '%s  %s\n' "$checksum" "/tmp/${asset}" | sha256sum -c -; \
    install -m 0755 "/tmp/${asset}" /out/zombie; \
    test -x /out/zombie; \
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

WORKDIR /src/zombie
COPY --from=contracts . /contracts
COPY --from=engine-go . /engine-go
COPY zombie/go.mod zombie/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    go mod download
COPY zombie ./
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    go build -trimpath -ldflags='-s -w' \
      -o /out/zombie-engine \
      ./cmd/zombie-engine

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
ARG ZOMBIE_VERSION
COPY --from=zombie-downloader /out/zombie /opt/lunafox-tools/bin/zombie
RUN set -eux; \
    printf '%s\n' "zombie=v1.3.0" > /opt/lunafox-tools/VERSIONS; \
    chmod 0755 /opt/lunafox-tools/bin/zombie
# lunafox:engine-owned:runtime-base:end

COPY --from=engine-builder /out/zombie-engine \
  /opt/lunafox-engine/bin/zombie-engine
RUN chmod 0755 /opt/lunafox-engine/bin/zombie-engine

FROM runtime-base AS verify
RUN --mount=type=bind,source=zombie/tests/container,target=/run/lunafox-conformance \
    /bin/sh /run/lunafox-conformance/container-conformance.sh \
      /opt/lunafox-engine/bin/zombie-engine \
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
ENTRYPOINT ["/opt/lunafox-engine/bin/zombie-engine"]
CMD []
# lunafox:generated:end
