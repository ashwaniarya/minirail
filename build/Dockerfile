FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/minirail ./cmd/minirail

FROM alpine:3.24
ARG TARGETARCH
ARG RAILPACK_VERSION=v0.40.1
ARG RAILPACK_SHA256_AMD64=2842de93e68713af9037e0bc0a398d7da78f3b96aa4804303a638db2bc69bd30
ARG RAILPACK_SHA256_ARM64=c24a064b586b8f4f8c2fab44dd5ef19253e4c6cc4e1df793b3ae19cd87f7a5d4
RUN case "$TARGETARCH" in \
      amd64) railpack_arch=x86_64 railpack_sha256="$RAILPACK_SHA256_AMD64" ;; \
      arm64) railpack_arch=arm64 railpack_sha256="$RAILPACK_SHA256_ARM64" ;; \
      *) echo "no railpack build for $TARGETARCH" >&2; exit 1 ;; \
    esac \
 && apk add --no-cache git ca-certificates \
 && wget -qO /tmp/railpack.tar.gz "https://github.com/railwayapp/railpack/releases/download/${RAILPACK_VERSION}/railpack-${RAILPACK_VERSION}-${railpack_arch}-unknown-linux-musl.tar.gz" \
 && echo "${railpack_sha256}  /tmp/railpack.tar.gz" | sha256sum -c - \
 && tar -xzf /tmp/railpack.tar.gz -C /usr/local/bin railpack \
 && rm /tmp/railpack.tar.gz
COPY --from=builder /out/minirail /usr/local/bin/minirail
USER 1000:1000
ENTRYPOINT ["minirail"]
