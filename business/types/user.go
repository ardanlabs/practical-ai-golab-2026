// Package types contains strong domain types shared across business packages.
package types

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	minUserNameLen = 1
	maxUserNameLen = 100
)

// UserName represents a validated user name.
type UserName string

// NewUserName validates the provided string and converts it into a UserName.
func NewUserName(s string) (UserName, error) {
	name := strings.TrimSpace(s)

	if utf8.RuneCountInString(name) < minUserNameLen {
		return "", fmt.Errorf("name cannot be empty")
	}

	if utf8.RuneCountInString(name) > maxUserNameLen {
		return "", fmt.Errorf("name cannot exceed %d characters", maxUserNameLen)
	}

	return UserName(name), nil
}
