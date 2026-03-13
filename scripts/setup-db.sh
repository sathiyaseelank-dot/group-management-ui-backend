#!/usr/bin/env bash
# setup-db.sh — Create the ZTNA PostgreSQL database and user
set -euo pipefail

DB_NAME="ztna"
DB_USER="ztnaadmin"
DB_PASS="inkztnapass"

echo "Setting up PostgreSQL for ZTNA..."

# Check postgres is running
if ! pg_isready -q 2>/dev/null; then
  echo "PostgreSQL is not running. Start it first:"
  echo "  sudo systemctl start postgresql   # Linux"
  echo "  brew services start postgresql@16 # macOS"
  exit 1
fi

# Create user (ignore if already exists)
sudo -u postgres psql -tc "SELECT 1 FROM pg_roles WHERE rolname='${DB_USER}'" |
  grep -q 1 && echo "User '${DB_USER}' already exists." ||
  sudo -u postgres psql -c "CREATE USER ${DB_USER} WITH PASSWORD '${DB_PASS}';"

# Create database (ignore if already exists)
sudo -u postgres psql -tc "SELECT 1 FROM pg_database WHERE datname='${DB_NAME}'" |
  grep -q 1 && echo "Database '${DB_NAME}' already exists." ||
  sudo -u postgres psql -c "CREATE DATABASE ${DB_NAME} OWNER ${DB_USER};"

# Grant privileges (PostgreSQL 15+ revokes public schema CREATE by default)
sudo -u postgres psql -c "GRANT ALL PRIVILEGES ON DATABASE ${DB_NAME} TO ${DB_USER};"
sudo -u postgres psql -d "${DB_NAME}" -c "GRANT ALL ON SCHEMA public TO ${DB_USER};"
sudo -u postgres psql -d "${DB_NAME}" -c "ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO ${DB_USER};"
sudo -u postgres psql -d "${DB_NAME}" -c "ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO ${DB_USER};"
sudo -u postgres psql -d "${DB_NAME}" -c "GRANT ALL ON ALL TABLES IN SCHEMA public TO ${DB_USER};"
sudo -u postgres psql -d "${DB_NAME}" -c "GRANT ALL ON ALL SEQUENCES IN SCHEMA public TO ${DB_USER};"

echo ""
echo "Done. Verify with:"
echo "  psql -h localhost -U ${DB_USER} -d ${DB_NAME}"
echo ""
echo "Connection string:"
echo "  DATABASE_URL=postgres://${DB_USER}:${DB_PASS}@localhost:5432/${DB_NAME}?sslmode=disable"
