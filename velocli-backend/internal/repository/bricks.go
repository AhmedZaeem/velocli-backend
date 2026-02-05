package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/velocli/velocli/velocli-backend/internal/domain"
)

type BricksRepository struct {
	pool *pgxpool.Pool
}

func NewBricksRepository(pool *pgxpool.Pool) *BricksRepository {
	return &BricksRepository{pool: pool}
}

func (r *BricksRepository) Insert(ctx context.Context, b domain.Brick) (string, error) {
	variablesJSON, err := json.Marshal(b.Variables)
	if err != nil {
		return "", err
	}

	var id string
	err = r.pool.QueryRow(ctx, `
		INSERT INTO bricks (name, version, encrypted_payload, variables, created_at, updated_at)
		VALUES ($1, $2, $3, $4::jsonb, now(), now())
		RETURNING id
	`, b.Name, b.Version, b.EncryptedPayload, variablesJSON).Scan(&id)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (r *BricksRepository) GetByNameVersion(ctx context.Context, name string, version string) (*domain.Brick, error) {
	var b domain.Brick
	var variablesJSON []byte
	err := r.pool.QueryRow(ctx, `
		SELECT id, name, version, encrypted_payload, variables, created_at, updated_at
		FROM bricks
		WHERE name = $1 AND version = $2
	`, name, version).Scan(&b.ID, &b.Name, &b.Version, &b.EncryptedPayload, &variablesJSON, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	_ = json.Unmarshal(variablesJSON, &b.Variables)
	return &b, nil
}

