#!/bin/bash

set -e

# ждем PostgreSQL
sleep 2


# создаем бд, если не сущ-ет
psql 'postgresql://validator:val1dat0r@localhost:5432/postgres' -c "CREATE DATABASE \"project-sem-1\";" 2>/dev/null || echo "Database already exists or can't create"

# создаем табл.
psql 'postgresql://validator:val1dat0r@localhost:5432/project-sem-1' -c "
CREATE TABLE IF NOT EXISTS prices (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    category TEXT NOT NULL,
    price DECIMAL(10,2) NOT NULL,
    create_date TIMESTAMP NOT NULL
);" 2>/dev/null || echo "Table checked or error"
