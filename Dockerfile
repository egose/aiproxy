# syntax=docker/dockerfile:1

FROM golang:1.26 AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/aiproxy \
    ./cmd/aiproxy

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /out/aiproxy /usr/local/bin/aiproxy

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/usr/local/bin/aiproxy", "serve", "--config", "/etc/aiproxy/config.hcl"]
