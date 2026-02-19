FROM golang:1.25.7-alpine AS go-builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download & go mod verify

COPY . .

RUN go test -c ./tests/integration -o integration-test

FROM alpine:3.22

WORKDIR /app

COPY --from=go-builder /app/integration-test ./integration-test
COPY --from=go-builder /app/configs ./configs

CMD ["./integration-test", "-test.v"]
