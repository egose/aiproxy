# syntax=docker/dockerfile:1@sha256:ecfaec9ed6d810b56388c508f4121597bfbba70d41a6dfeee4d8cad5f295fc32

FROM node:26-slim AS web-builder

WORKDIR /src
RUN npm install -g pnpm@11.24.0
COPY package.json pnpm-workspace.yaml pnpm-lock.yaml ./
COPY web-ui/package.json ./web-ui/package.json
COPY website/package.json ./website/package.json
RUN pnpm install --frozen-lockfile
COPY web-ui ./web-ui
RUN pnpm --filter @aiproxy/web-ui build

FROM golang:1.27.4@sha256:5e8afd493a7124a8586e134d9b532731161d4b9becf9dee5a5956b107426f93d AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=web-builder /src/internal/webui/dist ./internal/webui/dist
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath -buildvcs=false \
    -ldflags="-buildid= -s -w -X main.version=${VERSION}" \
    -o /out/aiproxy \
    ./cmd/aiproxy && mkdir -p /out/tls

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab

COPY --from=builder /out/aiproxy /usr/local/bin/aiproxy
COPY --from=builder /out/tls/ /usr/local/share/aiproxy/tls/
COPY --chmod=0444 examples/tls/*-fetch.json /usr/local/share/aiproxy/tls/

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/usr/local/bin/aiproxy", "serve", "--config", "/etc/aiproxy/config.hcl"]
