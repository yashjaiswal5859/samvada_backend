#!/bin/bash
# =============================================================================
# Samvada — PostgreSQL Multi-Database Initialization Script
# =============================================================================
# This script runs ONCE on first container boot (mounted in /docker-entrypoint-initdb.d/).
# It creates the second database (samvada_abs) since the default database
# (samvada_auth) is already created by the POSTGRES_DB environment variable.
#
# Databases created:
#   1. samvada_auth  — Auth Service (users, sessions, OTP, OAuth)
#   2. samvada_abs   — Application & Billing Service (orgs, agents, billing, etc.)
#
# All databases are owned by the `samvada` superuser.
# =============================================================================

set -e

echo "================================================"
echo "  Samvada PostgreSQL: Initializing databases..."
echo "================================================"

# The default database (samvada_auth) is already created by POSTGRES_DB env var.
# We only need to create the ABS database.

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL

    -- =========================================================================
    -- Create the ABS (Application & Billing Service) database
    -- =========================================================================
    SELECT 'Creating database: samvada_abs' AS status;
    CREATE DATABASE samvada_abs
        WITH OWNER = samvada
        ENCODING = 'UTF8';

    -- =========================================================================
    -- Create Ory Stack databases (Kratos, Hydra, Keto)
    -- =========================================================================
    SELECT 'Creating database: kratos' AS status;
    CREATE DATABASE kratos
        WITH OWNER = samvada
        ENCODING = 'UTF8';

    SELECT 'Creating database: hydra' AS status;
    CREATE DATABASE hydra
        WITH OWNER = samvada
        ENCODING = 'UTF8';

    SELECT 'Creating database: keto' AS status;
    CREATE DATABASE keto
        WITH OWNER = samvada
        ENCODING = 'UTF8';

    -- =========================================================================
    -- Enable required extensions on samvada_auth
    -- =========================================================================
    \c samvada_auth
    CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
    CREATE EXTENSION IF NOT EXISTS "pgcrypto";

EOSQL

# Connect to samvada_abs and enable extensions
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "samvada_abs" <<-EOSQL

    -- =========================================================================
    -- Enable required extensions on samvada_abs
    -- =========================================================================
    CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
    CREATE EXTENSION IF NOT EXISTS "pgcrypto";

    -- pgvector for Knowledge Base RAG embeddings (if available)
    -- CREATE EXTENSION IF NOT EXISTS "vector";

EOSQL

echo "================================================"
echo "  Databases initialized successfully:"
echo "    - samvada_auth  (Auth Service)"
echo "    - samvada_abs   (Application & Billing Service)"
echo "  Extensions: uuid-ossp, pgcrypto"
echo "================================================"
