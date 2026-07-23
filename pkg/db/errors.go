package db

import "errors"

var ErrMissingDatabaseURL = errors.New("DATABASE_URL is required (Neon Postgres connection string)")

var ErrCannotDeleteSharedList = errors.New("shared shopping lists can't be deleted")

var ErrInvalidShoppingListTitle = errors.New("title is required")
