// Package userbus contains business logic for maintaining users.
package userbus

import (
	"context"
	"fmt"
	"time"
	"uuid"

	"github.com/ardanlabs/practical-ai-golab-2026/business/types"
)

// User contains information about a user in the system.
type User struct {
	ID        uuid.UUID
	Name      types.UserName
	CreatedAt time.Time
}

// Store defines the capabilities that a storage implementation must
// support for the user domain.
type Store interface {
	FetchUsers(ctx context.Context) ([]User, error)
}

// Service maintains business logic for users in the system.
type Service struct {
	store Store
}

// New constructs a Service for working with users.
func New(store Store) *Service {
	return &Service{store: store}
}

// ListUsers returns all the users in the system.
func (s *Service) ListUsers(ctx context.Context) ([]User, error) {
	users, err := s.store.FetchUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing users: %w", err)
	}

	return users, nil
}
