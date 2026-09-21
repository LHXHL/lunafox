# syntax=docker/dockerfile:1.7

# lunafox:dockerfile-mode=standard
# lunafox:generated:begin
ARG ENGINE_GO_VERSION=1.26
ARG UBUNTU_BASE=public.ecr.aws/docker/library/ubuntu:noble-20260113
ARG ENGINE_IMAGE_VERSION
ARG ENGINE_IMAGE_SOURCE=https://github.com/LHXHL/lunafox
ARG GOPROXY=https://proxy.golang.org,direct

# lunafox:engine-owned:build:begin
ARG GOGO_VERSION=v2.15.0
FROM --platform=$BUILDPLATFORM public.ecr.aws/docker/library/alpine:3.23.2 AS gogo-downloader
ARG TARGETARCH
ARG GOGO_VERSION

WORKDIR /out
RUN set -eux; \
    case "$TARGETARCH" in \
      amd64) asset="gogo_linux_amd64"; checksum="3b51080a909023f7d484e2443740b572307ce0054eb9e32d98f0a06bfcd2db90" ;; \
      arm64) asset="gogo_linux_arm64"; checksum="f34e248bc1d0f8e18101655850082b9f48f5f9f5a102cbac107086f3798c20a9" ;; \
      *) echo "unsupported gogo target architecture: $TARGETARCH" >&2; exit 1 ;; \
    esac; \
    url="https://github.com/chainreactors/gogo/releases/download/${GOGO_VERSION}/${asset}"; \
    attempt=1; \
    until wget -T 60 -O "/tmp/${asset}" "$url"; do \
      test "$attempt" -lt 5; \
      attempt=$((attempt + 1)); \
      rm -f "/tmp/${asset}"; \
      sleep 2; \
    done; \
    printf '%s  %s\n' "$checksum" "/tmp/${asset}" | sha256sum -c -; \
    install -m 0755 "/tmp/${asset}" /out/gogo; \
    test -x /out/gogo; \
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

WORKDIR /src/gogo
COPY --from=contracts . /contracts
COPY --from=engine-go . /engine-go
COPY gogo/go.mod gogo/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    go mod download
COPY gogo ./
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    go build -trimpath -ldflags='-s -w' \
      -o /out/gogo-engine \
      ./cmd/gogo-engine

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
ARG GOGO_VERSION
COPY --from=gogo-downloader /out/gogo /opt/lunafox-tools/bin/gogo
RUN set -eux; \
    printf '%s\n' "gogo=v2.15.0" > /opt/lunafox-tools/VERSIONS; \
    chmod 0755 /opt/lunafox-tools/bin/gogo
# lunafox:engine-owned:runtime-base:end

COPY --from=engine-builder /out/gogo-engine \
  /opt/lunafox-engine/bin/gogo-engine
RUN chmod 0755 /opt/lunafox-engine/bin/gogo-engine

FROM runtime-base AS verify
RUN --mount=type=bind,source=gogo/tests/container,target=/run/lunafox-conformance \
    /bin/sh /run/lunafox-conformance/container-conformance.sh \
      /opt/lunafox-engine/bin/gogo-engine \
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
ENTRYPOINT ["/opt/lunafox-engine/bin/gogo-engine"]
CMD []
# lunafox:generated:end
