# --- Stage 1: The "Builder" ---
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY src/ ./src/
RUN CGO_ENABLED=0 go build -o /main ./src

# --- Stage 2: The Final Image ---
FROM alpine:latest
WORKDIR /
COPY --from=builder /main /main
CMD ["/main"]