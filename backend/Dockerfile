FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /sni-gateway ./cmd/server

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=builder /sni-gateway .

# Copy pre-built frontend static files
COPY public/ ./public/

EXPOSE 8080
CMD ["./sni-gateway"]
