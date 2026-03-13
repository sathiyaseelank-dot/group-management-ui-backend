-- init-db.sql — Runs automatically on first container start.
-- Grants the ztnaadmin user full access to the public schema.
-- (The database and user are already created by POSTGRES_USER/POSTGRES_DB env vars.)

GRANT ALL ON SCHEMA public TO ztnaadmin;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO ztnaadmin;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO ztnaadmin;
