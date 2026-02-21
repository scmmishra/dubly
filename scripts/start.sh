#!/usr/bin/env bash
set -euo pipefail

DB_PATH="${DUBLY_DB_PATH:-./dubly.db}"

echo "Building dubly..."
go build -o dubly ./cmd/server

# Restore from S3 if the database doesn't exist yet.
# -if-replica-exists exits 0 when no replica is found (first deploy).
if [ ! -f "$DB_PATH" ]; then
  echo "No local database found, attempting restore from S3..."
  litestream restore -config litestream.yml -if-replica-exists "$DB_PATH"
fi

echo "Starting dubly under litestream..."
exec litestream replicate -config litestream.yml -exec "./dubly"
