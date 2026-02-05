package domain

import "time"

type Brick struct {
	ID               string
	Name             string
	Version          string
	EncryptedPayload []byte
	Variables        []string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
