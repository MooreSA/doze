#!/bin/bash
# Test script for local mode (without SPRITES_API_KEY)

set -e

echo "🧪 Testing Doze API in local mode..."
echo ""

# Start the server in the background
cd api
echo "Starting API server..."
go run main.go &
SERVER_PID=$!

# Wait for server to start
sleep 2

# Trap to kill server on exit
trap "echo 'Stopping server...'; kill $SERVER_PID 2>/dev/null || true" EXIT

echo "Testing /health endpoint..."
curl -s http://localhost:2020/health | jq .

echo ""
echo "Testing /status endpoint..."
curl -s http://localhost:2020/status | jq .

echo ""
echo "✅ Basic endpoints work!"
echo ""
echo "To test /start and /message, you need Claude Code installed locally."
echo "The server is running in the foreground. Press Ctrl+C to stop."

# Wait for user interrupt
wait $SERVER_PID
