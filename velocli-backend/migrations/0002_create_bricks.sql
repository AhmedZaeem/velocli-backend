CREATE TABLE IF NOT EXISTS bricks (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	name TEXT NOT NULL,
	version TEXT NOT NULL,
	encrypted_payload BYTEA NOT NULL,
	variables JSONB NOT NULL DEFAULT '[]'::jsonb,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	UNIQUE (name, version)
);

CREATE INDEX IF NOT EXISTS bricks_name_idx ON bricks (name);
