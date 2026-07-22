-- recipes + shopping lists + pantry for recipe-app (per-user)
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE IF NOT EXISTS users (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	email TEXT NOT NULL,
	password_hash TEXT,
	google_sub TEXT,
	name TEXT NOT NULL DEFAULT '',
	avatar_url TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	CONSTRAINT users_email_unique UNIQUE (email),
	CONSTRAINT users_google_sub_unique UNIQUE (google_sub)
);

CREATE TABLE IF NOT EXISTS recipes (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	title TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	emoji TEXT NOT NULL DEFAULT '',
	ingredients TEXT[] NOT NULL DEFAULT '{}',
	steps TEXT[] NOT NULL DEFAULT '{}',
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS shopping_lists (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	title TEXT NOT NULL,
	emoji TEXT NOT NULL DEFAULT '🛒',
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS shopping_list_items (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	list_id UUID NOT NULL REFERENCES shopping_lists (id) ON DELETE CASCADE,
	text TEXT NOT NULL,
	checked BOOLEAN NOT NULL DEFAULT FALSE,
	source_recipe_id UUID REFERENCES recipes (id) ON DELETE SET NULL,
	sort_order INTEGER NOT NULL DEFAULT 0,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS pantry_items (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	name TEXT NOT NULL,
	emoji TEXT NOT NULL DEFAULT '',
	notes TEXT NOT NULL DEFAULT '',
	in_stock BOOLEAN NOT NULL DEFAULT TRUE,
	percent INTEGER NOT NULL DEFAULT 100,
	unit TEXT NOT NULL DEFAULT 'percent',
	sort_order INTEGER NOT NULL DEFAULT 0,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One-time: wipe shared pre-auth data and attach ownership columns.
DO $$
BEGIN
	-- recipes
	IF EXISTS (
		SELECT 1 FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'recipes'
	) AND NOT EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'recipes' AND column_name = 'user_id'
	) THEN
		TRUNCATE TABLE shopping_list_items, shopping_lists, recipes CASCADE;
		ALTER TABLE recipes
			ADD COLUMN user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE;
	END IF;

	-- shopping_lists
	IF EXISTS (
		SELECT 1 FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'shopping_lists'
	) AND NOT EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'shopping_lists' AND column_name = 'user_id'
	) THEN
		TRUNCATE TABLE shopping_list_items, shopping_lists CASCADE;
		ALTER TABLE shopping_lists
			ADD COLUMN user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE;
	END IF;

	-- pantry_items
	IF EXISTS (
		SELECT 1 FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'pantry_items'
	) AND NOT EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'pantry_items' AND column_name = 'user_id'
	) THEN
		TRUNCATE TABLE pantry_items CASCADE;
		ALTER TABLE pantry_items
			ADD COLUMN user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE;
	END IF;
END $$;

ALTER TABLE recipes ADD COLUMN IF NOT EXISTS emoji TEXT NOT NULL DEFAULT '';
ALTER TABLE shopping_lists ADD COLUMN IF NOT EXISTS emoji TEXT NOT NULL DEFAULT '🛒';
ALTER TABLE pantry_items ADD COLUMN IF NOT EXISTS percent INTEGER NOT NULL DEFAULT 100;
ALTER TABLE pantry_items ADD COLUMN IF NOT EXISTS unit TEXT NOT NULL DEFAULT 'percent';

-- Prefer whole-number counts for typical countable staples.
UPDATE pantry_items
SET
	unit = 'count',
	percent = CASE
		WHEN percent = 100 AND lower(name) = 'eggs' THEN 12
		WHEN percent = 100 AND lower(name) IN ('onion', 'onions', 'garlic') THEN 3
		WHEN percent = 100 AND lower(name) IN ('lemon', 'lemons') THEN 2
		ELSE percent
	END
WHERE unit = 'percent'
	AND lower(name) IN ('eggs', 'onion', 'onions', 'lemon', 'lemons', 'garlic');

CREATE INDEX IF NOT EXISTS shopping_list_items_list_id_idx
	ON shopping_list_items (list_id);

CREATE INDEX IF NOT EXISTS recipes_user_id_updated_at_idx
	ON recipes (user_id, updated_at DESC);

CREATE INDEX IF NOT EXISTS shopping_lists_user_id_updated_at_idx
	ON shopping_lists (user_id, updated_at DESC);

CREATE INDEX IF NOT EXISTS pantry_items_user_id_sort_order_idx
	ON pantry_items (user_id, sort_order ASC, name ASC);

CREATE INDEX IF NOT EXISTS recipes_updated_at_idx
	ON recipes (updated_at DESC);

CREATE INDEX IF NOT EXISTS shopping_lists_updated_at_idx
	ON shopping_lists (updated_at DESC);

CREATE INDEX IF NOT EXISTS pantry_items_sort_order_idx
	ON pantry_items (sort_order ASC, name ASC);

ALTER TABLE users ADD COLUMN IF NOT EXISTS kroger_zip TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS kroger_location_id TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS kroger_store_name TEXT NOT NULL DEFAULT '';

-- Recipe servings attached to a shopping list: { "recipe-uuid": 2, ... }
ALTER TABLE shopping_lists ADD COLUMN IF NOT EXISTS recipe_counts JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE TABLE IF NOT EXISTS friendships (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	requester_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	addressee_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	status TEXT NOT NULL DEFAULT 'pending',
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	CONSTRAINT friendships_no_self CHECK (requester_id <> addressee_id),
	CONSTRAINT friendships_status_check CHECK (status IN ('pending', 'accepted', 'declined')),
	CONSTRAINT friendships_pair_unique UNIQUE (requester_id, addressee_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS friendships_unordered_pair_idx
	ON friendships (LEAST(requester_id, addressee_id), GREATEST(requester_id, addressee_id));

CREATE INDEX IF NOT EXISTS friendships_requester_id_idx ON friendships (requester_id);
CREATE INDEX IF NOT EXISTS friendships_addressee_id_idx ON friendships (addressee_id);

CREATE TABLE IF NOT EXISTS notifications (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	type TEXT NOT NULL,
	title TEXT NOT NULL,
	body TEXT NOT NULL DEFAULT '',
	data JSONB NOT NULL DEFAULT '{}'::jsonb,
	read_at TIMESTAMPTZ,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS notifications_user_id_created_at_idx
	ON notifications (user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS notifications_user_id_unread_idx
	ON notifications (user_id)
	WHERE read_at IS NULL;

