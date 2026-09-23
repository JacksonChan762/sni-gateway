#!/usr/bin/env bash
set -e

cd "$(dirname "$0")/.."

echo "==> Starting backend and database..."
docker compose up -d --build --remove-orphans --wait postgres backend

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

trap 'kill "$FRONTEND_PID" 2>/dev/null || true' EXIT
wait
