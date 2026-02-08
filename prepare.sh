#!/bin/bash
# setup project

go mod tidy
go get github.com/lib/pq

# start postgres if not running
if ! docker ps | grep -q project-sem1-db; then
    docker stop project-sem1-db 2>/dev/null
    docker rm project-sem1-db 2>/dev/null
    
    docker run -d \
        --name project-sem1-db \
        -p 5432:5432 \
        -e POSTGRES_USER=validator \
        -e POSTGRES_PASSWORD=validator \
        -e POSTGRES_DB=project-sem-1 \
        postgres:15
    
    sleep 3
fi

# create table
docker exec project-sem1-db psql -U validator -d project-sem-1 -c "
CREATE TABLE IF NOT EXISTS prices (
    id SERIAL PRIMARY KEY,
    product_id INTEGER,
    name TEXT,
    category TEXT,
    price DECIMAL(10, 2),
    create_date DATE
);" 2>/dev/null || true

echo "ready"
