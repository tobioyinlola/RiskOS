#!/bin/sh
# Local development only: creates the application role (riskos_app) with a login.
#
# The api, worker and riskosctl connect as riskos_app, which owns no tables and has no
# BYPASSRLS, so row-level security always applies (docs/TENANCY.md). Migrations run as the
# owner (POSTGRES_USER) and grant riskos_app its privileges. In deployed environments the platform
# creates this role with a password from the secrets manager.
#
# Runs once, when the data volume is first initialized.
set -eu

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
  -v app_password="$POSTGRES_APP_PASSWORD" <<'SQL'
CREATE ROLE riskos_app LOGIN PASSWORD :'app_password' NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;
SQL
