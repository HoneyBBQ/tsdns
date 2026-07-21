FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY go.mod go.sum go.work ./
COPY core/go.mod ./core/go.mod
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

# Leverage Docker's default platform ARGs for cross-compilation
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown

# Cache dependencies and build artifacts to speed up subsequent builds
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
    -ldflags "-s -w -X 'github.com/honeybbq/tsdns/internal/cli.version=$VERSION' -X 'github.com/honeybbq/tsdns/internal/cli.commit=$COMMIT' -X 'github.com/honeybbq/tsdns/internal/cli.date=$DATE'" \
    -o /out/tsdns ./cmd/tsdns
RUN mkdir -p /out/data


FROM gcr.io/distroless/static-debian13:nonroot

WORKDIR /app

# Enable Unix Domain Socket by default for local management within the container
ENV TSDNS_API_SOCKET=/tmp/tsdns.sock

COPY --from=builder --chown=65532:65532 /out/tsdns /app/tsdns
COPY --from=builder --chown=65532:65532 /out/data /data

EXPOSE 41144/tcp
EXPOSE 8080/tcp

ENTRYPOINT ["/app/tsdns"]
CMD ["serve"]
