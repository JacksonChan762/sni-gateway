#!/usr/bin/env bash
set -e

cd "$(dirname "$0")/.."

echo "==> Building frontend..."
cd frontend
npm install
npm run build
cd ..

echo "==> Building backend..."
cd backend
go mod tidy
go build -o ../dist/sni-gateway ./cmd/server
cd ..

echo "==> Building relay image..."
docker build -t sni-gateway-relay:latest ./relay

echo "==> Done. Binary: dist/sni-gateway"
