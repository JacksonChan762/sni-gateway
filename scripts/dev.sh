#!/usr/bin/env bash
set -e

cd "$(dirname "$0")/.."

# Start postgres
docker compose up -d postgres

# Wait for it
echo "Waiting for postgres..."
until docker compose exec postgres pg_isready -U sni -d sni_gateway 2>/dev/null; do
  sleep 1
done

echo "==> Starting backend..."
cd backend
DATABASE_URL="postgres://sni:sni_secret@localhost:5432/sni_gateway?sslmode=disable" \
  PORT=8080 \
  NGINX_CONFIG_DIR=/tmp/nginx_conf \
  go run ./cmd/server &
BACKEND_PID=$!
cd ..

echo "==> Starting frontend dev server..."
cd frontend
npm run dev &
FRONTEND_PID=$!
cd ..

echo ""
echo "  Backend:  http://localhost:8080"
echo "  Frontend: http://localhost:5173"
echo ""
echo "Press Ctrl+C to stop."

trap "kill $BACKEND_PID $FRONTEND_PID 2>/dev/null; docker compose stop postgres" EXIT
wait
