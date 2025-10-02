FROM golang:1.25.0-alpine AS go-builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download && go mod verify

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -a -installsuffix cgo -o ./bin/order-service ./cmd/order-service/main.go

FROM alpine:3.22

COPY --from=go-builder /app/configs /app/configs
COPY --from=go-builder /app/migrations /app/migrations
COPY --from=go-builder /app/docs /app/docs
COPY --from=go-builder /app/web /web

COPY --from=go-builder /app/bin/order-service /order-service

ENTRYPOINT ["/order-service"]