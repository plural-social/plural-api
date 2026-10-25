ARG GO_VERSION=1.27

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION} AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY ./ ./

ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -o /executable ./cmd/plural-api-transcoder

# Not scratch: the transcoder runs ffmpeg and ffprobe.
FROM debian:trixie-slim AS final
RUN apt-get update && apt-get install -y --no-install-recommends ffmpeg ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=builder /executable /executable
USER nobody:nogroup
ENTRYPOINT ["/executable"]
