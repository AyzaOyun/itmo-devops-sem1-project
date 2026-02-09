#!/bin/bash

echo "=== Simulating GitHub Actions CI ==="

# 1. Запускаем PostgreSQL в Docker как в CI
echo "1. Starting PostgreSQL Docker container..."
docker run -d --name ci-postgres \
  -e POSTGRES_USER=validator \
  -e POSTGRES_PASSWORD=val1dat0r \
  -e POSTGRES_DB=project-sem-1 \
  -p 5432:5432 \
  postgres:15

# 2. Ждем пока PostgreSQL будет готов
echo "2. Waiting for PostgreSQL to be ready..."
for i in {1..30}; do
  if docker exec ci-postgres pg_isready -U validator 2>/dev/null; then
    echo "   PostgreSQL is ready after $i seconds"
    break
  fi
  sleep 1
  if [ $i -eq 30 ]; then
    echo "   ERROR: PostgreSQL failed to start in 30 seconds"
    docker logs ci-postgres
    exit 1
  fi
done

# 3. Выполняем prepare.sh
echo "3. Running prepare.sh..."
export POSTGRES_HOST=localhost
export POSTGRES_PORT=5432
export POSTGRES_DB=project-sem-1
export POSTGRES_USER=validator
export POSTGRES_PASSWORD=val1dat0r

chmod +x scripts/prepare.sh
./scripts/prepare.sh

# 4. Запускаем сервер в фоне
echo "4. Starting Go server..."
cd /home/ayza/itmo-sem1-project/itmo-devops-sem1-project-template
go run main.go > server.log 2>&1 &
SERVER_PID=$!

# 5. Ждем пока сервер запустится
echo "5. Waiting for Go server to start..."
for i in {1..30}; do
  if curl -s http://localhost:8080/api/v0/prices >/dev/null 2>&1; then
    echo "   Go server is ready after $i seconds"
    break
  fi
  sleep 1
  if [ $i -eq 30 ]; then
    echo "   ERROR: Go server failed to start in 30 seconds"
    cat server.log
    kill $SERVER_PID 2>/dev/null
    exit 1
  fi
done

# 6. Запускаем тесты
echo "6. Running tests..."
./scripts/tests.sh 1
TEST_RESULT=$?

# 7. Останавливаем все
echo "7. Cleaning up..."
kill $SERVER_PID 2>/dev/null
docker stop ci-postgres >/dev/null 2>&1
docker rm ci-postgres >/dev/null 2>&1
rm -f server.log

echo "=== Simulation complete ==="
exit $TEST_RESULT
