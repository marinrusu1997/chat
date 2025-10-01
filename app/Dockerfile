# --- Stage 1: The "Builder" ---
FROM golang:1.25-alpine AS builder
RUN apk update && apk add --no-cache build-base geos-dev pkgconf
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY src/ ./src/
RUN go build -o /main ./src

# --- Stage 2: The Final Image ---
FROM alpine:latest
RUN apk update && apk add --no-cache geos busybox-extras netcat-openbsd
WORKDIR /
COPY --from=builder /main /main
CMD ["/main"]