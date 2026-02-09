#!/bin/bash
set -e

# Запускаем сервер в фоне
go run main.go &
SERVER_PID=$!

echo $SERVER_PID > /tmp/server.pid

# Даем больше времени на запуск
sleep 10

# Простая проверка
if kill -0 $SERVER_PID 2>/dev/null; then
    exit 0
else
    exit 1
fi
