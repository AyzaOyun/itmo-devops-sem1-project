#!/bin/bash
set -e

cd ..
nohup go run main.go > server.log 2>&1 &
SERVER_PID=$!

echo "Server started with PID: $SERVER_PID"
echo "Logs: server.log"

sleep 15

if curl -s http://localhost:8080/api/v0/prices >/dev/null 2>&1; then
    echo $SERVER_PID > /tmp/server.pid
    exit 0
else
    echo "Server failed to start"
    cat server.log
    kill $SERVER_PID 2>/dev/null
    exit 1
fi
