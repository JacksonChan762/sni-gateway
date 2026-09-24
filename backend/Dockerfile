FROM node:22-alpine AS frontend
WORKDIR /build/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ .
RUN CGO_ENABLED=0 GOOS=linux go build -o /sni-gateway ./cmd/server

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=builder /sni-gateway .

# Build the UI from the same source revision as the backend
COPY --from=frontend /build/backend/public/ ./public/

COPY LICENSE THIRD_PARTY_NOTICES.md ./licenses/
COPY backend/internal/catalog/data/LICENSE.gluetun-servers ./licenses/
COPY backend/internal/provider/data/LICENSE.gluetun ./licenses/

EXPOSE 80 443 8080
CMD ["./sni-gateway"]
