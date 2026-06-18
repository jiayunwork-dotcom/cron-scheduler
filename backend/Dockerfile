FROM golang:1.21-alpine AS builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -o /app/server main.go

FROM alpine:latest
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /app/server /app/server
COPY --from=builder /app/sql /app/sql
EXPOSE 8080
CMD ["/app/server"]
