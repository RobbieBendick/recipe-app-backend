# recipe-app-backend

Go API for the recipe app. Neon Postgres. Deploys to Vercel as a Go serverless function.

## Local

```sh
cp .env.example .env
# set DATABASE_URL (or POSTGRES_URL_NON_POOLING)
go run ./cmd/api
```

Health: http://127.0.0.1:8080/health

## Vercel

Entrypoint: `api/index.go` + `vercel.json` (all routes rewrite to `/api`).

Neon/Vercel env vars are auto-detected in this order:

1. `DATABASE_URL`
2. `POSTGRES_URL_NON_POOLING`
3. `POSTGRES_URL`
4. `POSTGRES_PRISMA_URL`

Also set `ALLOWED_ORIGIN` to your frontend origin(s), comma-separated (or `*`).

Production URL: https://recipe-app-backend-bay.vercel.app/

## API

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health + DB ping |
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
