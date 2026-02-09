#!/bin/bash
set -e

# запускаем сервер
go run main.go &
SERVER_PID=$!

# Ждем запуска
sleep 3

# проверяем что сервер запущен
if ps -p $SERVER_PID > /dev/null; then
    echo "Server started with PID: $SERVER_PID"
    echo $SERVER_PID > /tmp/server.pid
else
    echo "Failed to start server"
    exit 1
fi
    