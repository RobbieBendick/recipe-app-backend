-- recipes + shopping lists + pantry for recipe-app
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE IF NOT EXISTS recipes (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	title TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	emoji TEXT NOT NULL DEFAULT '',
	ingredients TEXT[] NOT NULL DEFAULT '{}',
	steps TEXT[] NOT NULL DEFAULT '{}',
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE recipes ADD COLUMN IF NOT EXISTS emoji TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS shopping_lists (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	title TEXT NOT NULL,
	emoji TEXT NOT NULL DEFAULT '🛒',
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE shopping_lists ADD COLUMN IF NOT EXISTS emoji TEXT NOT NULL DEFAULT '🛒';

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
	name TEXT NOT NULL,
	emoji TEXT NOT NULL DEFAULT '',
	notes TEXT NOT NULL DEFAULT '',
	in_stock BOOLEAN NOT NULL DEFAULT TRUE,
	percent INTEGER NOT NULL DEFAULT 50,
	sort_order INTEGER NOT NULL DEFAULT 0,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE pantry_items ADD COLUMN IF NOT EXISTS percent INTEGER NOT NULL DEFAULT 50;

CREATE INDEX IF NOT EXISTS shopping_list_items_list_id_idx
	ON shopping_list_items (list_id);

CREATE INDEX IF NOT EXISTS recipes_updated_at_idx
	ON recipes (updated_at DESC);

CREATE INDEX IF NOT EXISTS shopping_lists_updated_at_idx
	ON shopping_lists (updated_at DESC);

CREATE INDEX IF NOT EXISTS pantry_items_sort_order_idx
	ON pantry_items (sort_order ASC, name ASC);
