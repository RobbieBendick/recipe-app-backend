package db

import "errors"

var ErrMissingDatabaseURL = errors.New("DATABASE_URL is required (Neon Postgres connection string)")
