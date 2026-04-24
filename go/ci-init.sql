-- ci-init.sql — run by Postgres on first boot via docker-entrypoint-initdb.d.
-- Creates additional databases needed by decomposed Go services.
-- ecommercedb is already created by POSTGRES_DB env var.

CREATE DATABASE productdb;
CREATE DATABASE cartdb;

-- Grant access to the default user
GRANT ALL PRIVILEGES ON DATABASE productdb TO taskuser;
GRANT ALL PRIVILEGES ON DATABASE cartdb TO taskuser;
