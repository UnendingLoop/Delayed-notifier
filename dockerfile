FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o delayed-notifier ./main.go

FROM alpine:3.18
WORKDIR /app
COPY --from=builder /app/delayed-notifier .
COPY .env .
COPY internal/web /app/internal/web
COPY internal/migration/0001_create_notifications_table.up.sql /app/migration/
EXPOSE 8080
CMD ["./delayed-notifier"]
