FROM golang:1.25-alpine AS builder

WORKDIR /app

# Dependencies first (cached layer)
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build static binary
RUN CGO_ENABLED=0 go build -o /server ./cmd/server

# --- Runtime ---
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /server /server
COPY config/local.yaml /config/local.yaml

ENV PORT=8080
ENV UPLOAD_DIR=/data/uploads
ENV CONFIG_PATH=/config/local.yaml
ENV SQLITE_PATH=/data/agent_runs.db

RUN mkdir -p /data/uploads

EXPOSE 8080

CMD ["/server"]
