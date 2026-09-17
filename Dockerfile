FROM oven/bun:1.4.0@sha256:5ff609364c049b54eb0ff560ec96319729a972078ef2c755d758f0c6ef89c2d6 AS builder

WORKDIR /build/web
COPY web/package.json web/bun.lock ./
RUN bun install --frozen-lockfile
COPY ./web ./
COPY ./VERSION /build/VERSION
RUN DISABLE_ESLINT_PLUGIN='true' VITE_REACT_APP_VERSION=$(cat /build/VERSION) bun run build

FROM golang:1.26.1-alpine@sha256:2389ebfa5b7f43eeafbd6be0c3700cc46690ef842ad962f6c5bd6be49ed82039 AS builder2
ENV GO111MODULE=on CGO_ENABLED=0 GOWORK=off

ARG TARGETOS
ARG TARGETARCH
ARG BUILD_COMMIT=unknown
ENV GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64}
ENV GOEXPERIMENT=greenteagc

WORKDIR /build

ADD go.mod go.sum ./
# relaykit is a local submodule referenced via replace; its go.mod must be
# present for go mod download to resolve the main module graph.
ADD relaykit/go.mod ./relaykit/go.mod
RUN go mod download

COPY . .
COPY --from=builder /build/web/dist ./web/dist
RUN BUILD_VERSION=$(cat VERSION) \
    && if [ -z "$BUILD_VERSION" ]; then BUILD_VERSION=v0.0.0; fi \
    && go build -ldflags "-s -w -X 'github.com/QuantumNous/new-api/common.Version=$BUILD_VERSION' -X 'github.com/QuantumNous/new-api/common.BuildVersion=$BUILD_VERSION' -X 'github.com/QuantumNous/new-api/common.BuildCommit=$BUILD_COMMIT'" -o new-api \
    && go build -trimpath -ldflags "-s -w" -o new-api-launcher ./cmd/backend-launcher

FROM debian:bookworm-slim@sha256:f06537653ac770703bc45b4b113475bd402f451e85223f0f2837acbf89ab020a

# WireGuard userspace tooling: hosts without the kernel module (for example
# CentOS 7) need wireguard-go for wg-quick's userspace fallback, and openresolv
# provides resolvconf without the debconf postinst that fails during builds.
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata libasan8 wget \
       wireguard-tools wireguard-go iproute2 openresolv \
    && rm -rf /var/lib/apt/lists/* \
    && update-ca-certificates

COPY --from=builder2 /build/new-api /usr/local/lib/new-api/new-api
COPY --from=builder2 /build/new-api-launcher /usr/local/bin/new-api-launcher
COPY LICENSE NOTICE THIRD-PARTY-LICENSES.md /licenses/
RUN ln -s /usr/local/lib/new-api/new-api /new-api
EXPOSE 3000
WORKDIR /data
ENTRYPOINT ["/usr/local/bin/new-api-launcher"]
