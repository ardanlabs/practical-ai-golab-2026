// Package api contains support code for building HTTP APIs.
package api

import (
	"encoding/json/v2"
	"fmt"
	"net/http"
)

// WriteJSON marshals v and writes it to the response with the
// provided status code.
func WriteJSON(w http.ResponseWriter, status int, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshaling response: %w", err)
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	if _, err := w.Write(b); err != nil {
		return fmt.Errorf("writing response: %w", err)
	}

	return nil
}

// WriteText writes a plain text message to the response with the
// provided status code.
func WriteText(w http.ResponseWriter, status int, msg string) error {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)

	if _, err := w.Write([]byte(msg)); err != nil {
		return fmt.Errorf("writing response: %w", err)
	}

	return nil
}
