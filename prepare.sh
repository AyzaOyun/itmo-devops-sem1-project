#!/bin/bash
# Prepare environment

# Install Go dependencies
go mod download

# ожидание
echo "Waiting for PostgreSQL..."
for i in {1..30}; do
  if pg_isready -h localhost -p 5432 -U validator 2>/dev/null; then
    echo "PostgreSQL is ready"
    break
  fi
  sleep 1
done

PGPASSWORD=val1dat0r psql -h localhost -U validator -d project-sem-1 -c "
CREATE TABLE IF NOT EXISTS prices (
    id SERIAL PRIMARY KEY,
    product_id INTEGER,
    name TEXT,
    category TEXT,
    price DECIMAL(10, 2),
    create_date DATE
);" 2>/dev/null || true
