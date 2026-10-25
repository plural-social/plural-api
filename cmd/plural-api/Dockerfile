ARG GO_VERSION=1.27

# Debian-based (not alpine) so tests can run with -race from the builder stage.
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION} AS builder

RUN mkdir /user && \
    echo 'nobody:x:65534:65534:nobody:/:' > /user/passwd && \
    echo 'nobody:x:65534:' > /user/group

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY ./ ./

ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -o /executable ./cmd/plural-api

FROM scratch AS final
COPY --from=builder /user/group /user/passwd /etc/
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /executable /executable
EXPOSE 4100
USER nobody:nobody
ENTRYPOINT ["/executable"]
