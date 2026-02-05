CREATE EXTENSION IF NOT EXISTS pgcrypto;

DO $$
BEGIN
	IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'subscription_status') THEN
		CREATE TYPE subscription_status AS ENUM ('active', 'past_due', 'cancelled');
	END IF;
END
$$;

CREATE TABLE IF NOT EXISTS customers (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	lemon_customer_id TEXT NOT NULL,
	license_key TEXT NOT NULL,
	subscription_status subscription_status NOT NULL DEFAULT 'cancelled',
	expires_at TIMESTAMPTZ NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS customers_license_key_idx ON customers (license_key);
CREATE UNIQUE INDEX IF NOT EXISTS customers_lemon_customer_id_uidx ON customers (lemon_customer_id);
