#!/bin/bash

echo "=== PostgreSQL Diagnosis ==="

echo "1. PostgreSQL service:"
sudo systemctl status postgresql --no-pager | head -5

echo -e "\n2. PostgreSQL port:"
sudo ss -tlnp | grep 5432 || sudo netstat -tlnp | grep 5432 || echo "Cannot check port"

echo -e "\n3. Current user in session:"
whoami

echo -e "\n4. PostgreSQL users:"
sudo -u postgres psql -c "\du" 2>/dev/null || echo "Cannot access PostgreSQL"

echo -e "\n5. Testing different connection methods:"

echo "Method A: Direct connection string"
psql 'postgresql://validator:val1dat0r@localhost:5432/project-sem-1' -c "SELECT 'Method A works';" 2>&1 | grep -v "SELECT"

echo -e "\nMethod B: Using PGPASSWORD"
PGPASSWORD=val1dat0r psql -U validator -h localhost -d "project-sem-1" -c "SELECT 'Method B works';" 2>&1 | grep -v "SELECT"

echo -e "\nMethod C: As postgres user"
sudo -u postgres psql -d "project-sem-1" -c "SELECT 'Method C works';" 2>&1 | grep -v "SELECT"

echo -e "\n6. Testing different passwords:"
for pass in 'val1dat0r' 'validator' 'password' 'test123' '123456' ''; do
    echo -n "Password '$pass': "
    PGPASSWORD=$pass psql -U validator -h localhost -d "project-sem-1" -c "SELECT 1;" 2>&1 | grep -q "1 row" && echo "✓" || echo "✗"
done

echo -e "\n7. pg_hba.conf authentication methods:"
sudo grep -E "(local|host).*all.*all" /etc/postgresql/16/main/pg_hba.conf

echo -e "\n8. Checking if password hash is set:"
sudo -u postgres psql -c "SELECT usename, passwd IS NOT NULL as has_password FROM pg_shadow WHERE usename = 'validator';" 2>/dev/null || echo "Cannot check password hash"
