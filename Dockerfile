FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /agenthub ./cmd/server

FROM alpine:3.19
RUN apk add --no-cache ca-certificates curl

WORKDIR /app
COPY --from=builder /agenthub .
COPY internal/db/migrations.sql /app/migrations.sql

EXPOSE 8081

CMD ["./agenthub"]
