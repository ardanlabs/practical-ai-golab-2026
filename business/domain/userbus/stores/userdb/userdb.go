// Package userdb manages user data in a PostgreSQL database.
package userdb

import (
	"context"
	"fmt"
	"time"
	"uuid"

	"github.com/jmoiron/sqlx"

	"github.com/ardanlabs/practical-ai-golab-2026/business/domain/userbus"
	"github.com/ardanlabs/practical-ai-golab-2026/business/types"
)

// dbUser represents a row in the users table.
type dbUser struct {
	ID        uuid.UUID `db:"id"`
	Name      string    `db:"name"`
	CreatedAt time.Time `db:"created_at"`
}

// toBus converts a database row into a business value.
func toBus(u dbUser) (userbus.User, error) {
	name, err := types.NewUserName(u.Name)
	if err != nil {
		return userbus.User{}, fmt.Errorf("converting user[%s]: %w", u.ID, err)
	}

	return userbus.User{
		ID:        u.ID,
		Name:      name,
		CreatedAt: u.CreatedAt,
	}, nil
}

// Store manages users in the PostgreSQL database.
type Store struct {
	db *sqlx.DB
}

// New constructs a Store for working with users.
func New(db *sqlx.DB) *Store {
	return &Store{db: db}
}

const fetchUsersSQL = `
	SELECT id, name, created_at
	FROM users
	ORDER BY created_at`

// FetchUsers returns all the users in the system.
func (s *Store) FetchUsers(ctx context.Context) ([]userbus.User, error) {
	var dbUsers []dbUser
	if err := s.db.SelectContext(ctx, &dbUsers, fetchUsersSQL); err != nil {
		return nil, fmt.Errorf("querying users: %w", err)
	}

	users := make([]userbus.User, 0, len(dbUsers))
	for _, row := range dbUsers {
		user, err := toBus(row)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}

	return users, nil
}
