FROM golang:1.27-alpine AS builder

WORKDIR /app

RUN apk add --no-cache ca-certificates git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/bin/url-shortener ./cmd/url-shortener

FROM alpine:3.21

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /app/bin/url-shortener /app/url-shortener
COPY config /app/config
COPY migrations /app/migrations

EXPOSE 8082

CMD ["/app/url-shortener"]
