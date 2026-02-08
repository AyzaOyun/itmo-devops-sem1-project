#!/bin/bash
set -e
sleep 3

# Проверяем подключение к БД
if ! psql 'postgresql://validator:val1dat0r@localhost:5432/project-sem-1' -c '\q' 2>/dev/null; then
    echo "ERROR: PostgreSQL not available"
    exit 1
fi

cd ..
go run main.go &
SERVER_PID=$!

sleep 2
echo $SERVER_PID > /tmp/server.pid
echo "Server started with PID $SERVER_PID"