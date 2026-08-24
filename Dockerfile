# syntax=docker/dockerfile:1@sha256:ecfaec9ed6d810b56388c508f4121597bfbba70d41a6dfeee4d8cad5f295fc32

FROM golang:1.26.6@sha256:0d1d3a794be25f809dd2cb3160d8c73276c4056a9f8242a138e908ddeee7b6b6 AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath -buildvcs=false \
    -ldflags="-buildid= -s -w -X main.version=${VERSION}" \
    -o /out/aiproxy \
    ./cmd/aiproxy

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab

COPY --from=builder /out/aiproxy /usr/local/bin/aiproxy

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/usr/local/bin/aiproxy", "serve", "--config", "/etc/aiproxy/config.hcl"]
