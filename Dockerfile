# Build Stage
FROM golang:1.25.5-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -v -o precious-time-tracker ./cmd/server

# Run Stage
FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/precious-time-tracker .
# Copy templates and static files if they are needed at runtime
COPY --from=builder /app/templates ./templates
COPY --from=builder /app/static ./static

EXPOSE 8080

CMD ["./precious-time-tracker"]
