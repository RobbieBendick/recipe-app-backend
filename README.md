# recipe-app-backend

Go API for the recipe app. Talks to Neon Postgres.

## Setup

1. Create a Neon project and copy the connection string.
2. Copy env file and fill in `DATABASE_URL`:

```sh
cp .env.example .env
```

3. Install deps and run:

```sh
go mod tidy
go run ./cmd/api
```

Health check: [http://127.0.0.1:8080/health](http://127.0.0.1:8080/health)

Schema is applied automatically on startup (`IF NOT EXISTS`).

## API

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/recipes` | List recipes |
| POST | `/api/recipes` | Create recipe |
| GET | `/api/recipes/{id}` | Get recipe |
| PUT | `/api/recipes/{id}` | Update recipe |
| DELETE | `/api/recipes/{id}` | Delete recipe |
| POST | `/api/recipes/{id}/shopping-list` | New list from one recipe |
| GET | `/api/shopping-lists` | List shopping lists |
| POST | `/api/shopping-lists` | Create shopping list |
| GET | `/api/shopping-lists/{id}` | Get list + items |
| PUT | `/api/shopping-lists/{id}` | Replace title + items |
| DELETE | `/api/shopping-lists/{id}` | Delete list |
| POST | `/api/shopping-lists/{id}/items` | Add item `{ "text": "Milk" }` |
| PATCH | `/api/shopping-lists/{id}/items/{itemId}` | Toggle checked |
| DELETE | `/api/shopping-lists/{id}/items/{itemId}` | Remove item |
| POST | `/api/shopping-lists/{id}/recipes` | Append recipe ingredients `{ "recipeId": "..." }` |

`POST .../recipes` is the multi-recipe flow (tacos + burgers → one list).

## Example

```sh
curl -s http://127.0.0.1:8080/api/recipes \
  -H 'Content-Type: application/json' \
  -d '{"title":"Tacos","description":"","ingredients":["tortillas","beef"],"steps":["Cook beef"]}'
```
