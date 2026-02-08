go mod tidy
go get github.com/lib/pq

# Wait for PostgreSQL if running in CI
if command -v pg_isready &> /dev/null; then
    echo "Waiting for PostgreSQL..."
    for i in {1..10}; do
        if pg_isready -h localhost -p 5432 -U validator 2>/dev/null; then
            echo "PostgreSQL ready"
            break
        fi
        sleep 1
    done
fi

# Start PostgreSQL container if not running (for local development)
if ! docker ps | grep -q postgres; then
    echo "Starting PostgreSQL container..."
    docker run -d \
        --name postgres \
        -p 5432:5432 \
        -e POSTGRES_USER=validator \
        -e POSTGRES_PASSWORD=val1dat0r \
        -e POSTGRES_DB=project-sem-1 \
        postgres:15 2>/dev/null || true
    sleep 3
fi

# Create table with correct structure
PGPASSWORD=val1dat0r psql -h localhost -U validator -d project-sem-1 -c "
CREATE TABLE IF NOT EXISTS prices (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    category TEXT NOT NULL,
    price DECIMAL(10, 2) NOT NULL,
    create_date TIMESTAMP NOT NULL
);" 2>/dev/null || echo "Table check completed"
