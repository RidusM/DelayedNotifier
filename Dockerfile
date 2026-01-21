# Stage 1: Build
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /delayed-notifier ./cmd/delayed-notifier/main.go

FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

RUN adduser -D -u 1000 appuser
USER appuser

WORKDIR /home/appuser

COPY --from=builder /delayed-notifier .

COPY --from=builder /app/config ./config

ENV CONFIG_PATH=./config/dev.env

CMD ["./delayed-notifier"]
