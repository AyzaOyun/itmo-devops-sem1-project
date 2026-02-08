#!/bin/bash
set -e

# Запускаем сервер в фоне
cd ..
go run main.go &
SERVER_PID=$!

# Ждем немного
sleep 2

# Сохраняем PID для остановки позже
echo $SERVER_PID > /tmp/server.pid