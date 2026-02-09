#!/bin/bash
set -e

# Ждем PostgreSQL
sleep 2

psql 'postgresql://validator:val1dat0r@localhost:5432/project-sem-1' -c "
CREATE TABLE IF NOT EXISTS prices (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    category TEXT NOT NULL,
    price DECIMAL(10,2) NOT NULL,
    create_date TIMESTAMP NOT NULL
);" 2>/dev/null || echo "Table checked"
