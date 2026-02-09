#!/bin/bash

echo "=== PostgreSQL Status Check ==="

echo "1. Systemd status:"
sudo systemctl status postgresql --no-pager

echo -e "\n2. Processes:"
ps aux | grep -E "postgres|postmaster" | grep -v grep

echo -e "\n3. Port 5432:"
sudo ss -tlnp | grep 5432 || sudo lsof -i :5432 2>/dev/null || echo "Not listening"

echo -e "\n4. Socket directory:"
ls -la /var/run/postgresql/ 2>/dev/null || echo "Socket directory not found"

echo -e "\n5. PostgreSQL logs:"
sudo tail -10 /var/log/postgresql/postgresql-16-main.log 2>/dev/null || echo "Log file not found"

echo -e "\n6. Trying to start manually..."
if ! ps aux | grep -q "[p]ostgres.*main"; then
    echo "PostgreSQL not running, starting..."
    sudo -u postgres /usr/lib/postgresql/16/bin/pg_ctl -D /var/lib/postgresql/16/main -l /tmp/postgres_start.log start
    sleep 3
    echo "Start log:"
    tail -5 /tmp/postgres_start.log 2>/dev/null || echo "No log file"
else
    echo "PostgreSQL is already running"
fi

echo -e "\n7. Testing connection..."
psql 'postgresql://validator:val1dat0r@localhost:5432/project-sem-1' -c "SELECT 1;" 2>&1 | tail -1
