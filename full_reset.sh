#!/bin/bash

echo "=== Full PostgreSQL Reset ==="

echo "1. Stopping PostgreSQL..."
sudo systemctl stop postgresql 2>/dev/null

echo "2. Creating correct pg_hba.conf..."
sudo tee /etc/postgresql/16/main/pg_hba.conf > /dev/null << 'CONFIG'
# PostgreSQL Client Authentication Configuration File
# ===================================================

# TYPE  DATABASE        USER            ADDRESS                 METHOD

# Database administrative login by Unix domain socket
local   all             postgres                                peer

# "local" is for Unix domain socket connections only
local   all             all                                     md5

# IPv4 local connections:
host    all             all             127.0.0.1/32            md5

# IPv6 local connections:
host    all             all             ::1/128                 md5

# Allow replication connections from localhost, by a user with the
# replication privilege.
local   replication     all                                     peer
host    replication     all             127.0.0.1/32            md5
host    replication     all             ::1/128                 md5
CONFIG

echo "3. Starting PostgreSQL..."
sudo systemctl start postgresql
sleep 3

echo "4. Resetting user and database..."
sudo -u postgres psql << "SQL" 2>/dev/null
\c postgres
DROP DATABASE IF EXISTS "project-sem-1";
DROP USER IF EXISTS validator;
CREATE USER validator WITH PASSWORD 'val1dat0r';
ALTER USER validator WITH SUPERUSER;
CREATE DATABASE "project-sem-1" WITH OWNER validator;
GRANT ALL PRIVILEGES ON DATABASE "project-sem-1" TO validator;
SQL

echo "5. Testing connection with 'val1dat0r'..."
if psql 'postgresql://validator:val1dat0r@localhost:5432/project-sem-1' -c "SELECT 1;" 2>/dev/null; then
    echo "✓ Success with 'val1dat0r'!"
else
    echo "✗ Failed with 'val1dat0r', trying 'password'..."
    sudo -u postgres psql -c "ALTER USER validator WITH PASSWORD 'password';" 2>/dev/null
    if psql 'postgresql://validator:password@localhost:5432/project-sem-1' -c "SELECT 1;" 2>/dev/null; then
        echo "✓ Success with 'password'!"
        echo "Updating project files to use 'password'..."
        sed -i "s/val1dat0r/password/g" main.go 2>/dev/null
        sed -i "s/val1dat0r/password/g" scripts/prepare.sh 2>/dev/null
        sed -i 's/DB_PASSWORD="val1dat0r"/DB_PASSWORD="password"/g' scripts/tests.sh 2>/dev/null
    else
        echo "✗ Still failing. PostgreSQL might not be running."
        sudo systemctl status postgresql
    fi
fi

echo -e "\n6. Final status:"
sudo ss -tlnp | grep 5432 || echo "Port 5432 not listening"
