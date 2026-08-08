# syntax=docker/dockerfile:1.7

FROM node:22.22.2-alpine AS web-build
WORKDIR /src/web
RUN corepack enable && corepack prepare pnpm@10.34.1 --activate
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build

FROM golang:1.26.5-alpine AS go-build
WORKDIR /src
RUN apk add --no-cache ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate

FROM alpine:3.23.3 AS runtime
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S -g 10001 keebhub \
    && adduser -S -D -H -u 10001 -G keebhub keebhub
WORKDIR /app
COPY --from=go-build --chown=10001:10001 /out/server /app/server
COPY --from=go-build --chown=10001:10001 /out/migrate /app/migrate
COPY --from=web-build --chown=10001:10001 /src/web/dist /app/web/dist
COPY --chown=10001:10001 db/migrations /app/db/migrations
USER 10001:10001
EXPOSE 8080
ENV HTTP_ADDR=:8080 \
    STATIC_DIR=/app/web/dist \
    MIGRATIONS_DIR=/app/db/migrations
ENTRYPOINT ["/app/server"]
