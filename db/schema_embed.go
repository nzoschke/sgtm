package db

import _ "embed"

// Schema is the SQLite schema used both by sqlc and runtime initialization.
//
//go:embed schema.sql
var Schema string
