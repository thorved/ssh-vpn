FROM oven/bun:1.3-alpine AS frontend-builder

WORKDIR /app/frontend
COPY frontend/package.json frontend/bun.lock ./
RUN bun install --frozen-lockfile

COPY frontend/ ./
RUN bun run build

FROM golang:1.25-alpine AS backend-builder

WORKDIR /app/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ ./
RUN GOOS=linux GOARCH=amd64 go build -o server ./cmd/server

FROM alpine:3.22

WORKDIR /app

RUN apk add --no-cache ca-certificates && mkdir -p /app/data

COPY --from=backend-builder /app/backend/server ./server
COPY --from=frontend-builder /app/frontend/out ./web

ENV SSH_HOST_KEY_PATH=/app/data/host_key
ENV WEB_STATIC_DIR=/app/web

EXPOSE 2222

CMD ["./server"]
