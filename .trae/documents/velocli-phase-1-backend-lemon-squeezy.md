# VeloCli (Phase 1) — Backend + Lemon Squeezy

## Architecture Acknowledgement
- VeloCli is a secure content-delivery system: encrypted “bricks” live server-side; CLI only receives them after subscription/license validation.
- Lemon Squeezy is the source of truth for licensing; backend is the gatekeeper that validates keys, tracks customer status locally, and serves encrypted bricks only to active customers.

## Repo Initialization (Monorepo)
- Create folders: `velocli-backend/`, `velocli-cli/`, `velocli-admin/`.
- Initialize Git at repo root and commit each step with semantic messages.

## Backend Project Layout (Clean-ish Layers)
- `velocli-backend/cmd/api/` — Fiber bootstrap (main).
- `velocli-backend/internal/config/` — env config loader.
- `velocli-backend/internal/http/` — router + handlers + middleware (JWT + webhook signature verification).
- `velocli-backend/internal/domain/` — domain models (Customer, Brick/Template metadata).
- `velocli-backend/internal/service/` — LemonSqueezyService + auth issuance.
- `velocli-backend/internal/repository/` — PostgreSQL access (customers, bricks).
- `velocli-backend/migrations/` — SQL migrations.

## Environment Variables (Phase 1)
- `DATABASE_URL`
- `JWT_SIGNING_KEY`
- `LEMON_SQUEEZY_API_KEY`
- `LEMON_SQUEEZY_STORE_ID` (and optionally `LEMON_SQUEEZY_PRODUCT_ID` / `LEMON_SQUEEZY_VARIANT_ID` for hard validation)
- `LEMON_WEBHOOK_SECRET`

## Data Model + Migration
- Add PostgreSQL migration to create:
  - `customers` table per requirement:
    - `id UUID PRIMARY KEY`
    - `lemon_customer_id TEXT NOT NULL`
    - `license_key TEXT NOT NULL` (store an HMAC digest of the license key so DB never stores the raw key; still a string and indexed)
    - `subscription_status TEXT` constrained to `active|past_due|cancelled`
    - `expires_at TIMESTAMPTZ NULL`
    - indexes on `license_key`, `lemon_customer_id`

## Lemon Squeezy Webhooks (Structs + Handler)
- Define Go structs matching Lemon’s webhook envelope: `meta.event_name`, optional `meta.custom_data`, and `data` resource object.
- Add focused typed structs for these events:
  - `subscription_created`
  - `subscription_updated`
  - `license_key_created`
- Implement `POST /webhooks/lemon`:
  - Verify `X-Signature` using `LEMON_WEBHOOK_SECRET` (webhooks are signed; verify before parsing/processing). (Docs: webhook requests include `X-Event-Name` and `X-Signature`.)
  - Switch on `meta.event_name` / `X-Event-Name`.
  - Upsert customer row based on Lemon identifiers and license key info where available.

## LemonSqueezyService (License Validation)
- Implement `LemonSqueezyService.ValidateLicense(key string) (bool, error)`.
- Use Lemon’s License API `POST https://api.lemonsqueezy.com/v1/licenses/validate` with `Accept: application/json` and `application/x-www-form-urlencoded`. Return `valid` boolean from response. (Docs: license validation/activation endpoints and response shape.)
- Optionally add hard checks for `store_id` and/or `product_id`/`variant_id` from the response meta to prevent accepting keys from other products.

## Auth/Login + Protected Brick Delivery
- Implement `POST /auth/login`:
  - Accept `{ "license_key": "..." }`.
  - Call `ValidateLicense`.
  - If valid, issue a JWT for CLI sessions and persist/update customer status locally.
- Implement `GET /bricks/:brick_id`:
  - Require JWT.
  - Check local customer status is `active` and `expires_at` not in the past.
  - Return encrypted brick payload (Phase 1 can return a stub until bricks storage is implemented, but route + auth + status checks should exist).

## Git Discipline (Planned Commits)
- `chore: init monorepo structure`
- `feat(backend): bootstrap fiber api skeleton`
- `feat(backend): add customers migration`
- `feat(backend): add lemon squeezy webhook structs and handler`
- `feat(backend): implement lemon squeezy license validation service`
- `feat(backend): add auth login and jwt middleware`
- `feat(backend): add bricks endpoint with access checks`

## Notes / Security Decisions
- Do not store raw license keys; store an HMAC digest in `customers.license_key` (still satisfies “string, indexed”).
- Verify webhook signatures before any state change.
- Validate Lemon response IDs (store/product/variant) to prevent cross-product key abuse.

## Sources
- Lemon Squeezy webhooks structure and signature headers: https://docs.lemonsqueezy.com/help/webhooks/webhook-requests and webhook guide example payloads.
- Lemon Squeezy License API validate endpoint: https://docs.lemonsqueezy.com/guides/tutorials/license-keys and License API docs.
