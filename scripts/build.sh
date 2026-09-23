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
mkdir -p ../dist
go build -o ../dist/sni-gateway ./cmd/server
cd ..

echo "==> Done. Binary: dist/sni-gateway"
