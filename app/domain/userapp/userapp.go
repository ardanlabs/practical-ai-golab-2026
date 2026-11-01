// Package userapp contains application logic for working with users.
package userapp

import (
	"log/slog"
	"net/http"
	"time"
	"uuid"

	"github.com/ardanlabs/practical-ai-golab-2026/app/sdk/api"
	"github.com/ardanlabs/practical-ai-golab-2026/business/domain/userbus"
)

// userResponse is the API representation of a user.
type userResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// listUsersResponse is the API representation of the list users endpoint.
type listUsersResponse struct {
	Users []userResponse `json:"users"`
}

// fromBusUsersResponse converts business values into API responses.
func fromBusUsersResponse(users []userbus.User) listUsersResponse {
	resp := listUsersResponse{
		Users: make([]userResponse, 0, len(users)),
	}

	for _, user := range users {
		resp.Users = append(resp.Users, userResponse{
			ID:        user.ID,
			Name:      string(user.Name),
			CreatedAt: user.CreatedAt,
		})
	}

	return resp
}

// Handlers groups the HTTP handlers for the user domain.
type Handlers struct {
	service *userbus.Service
}

// NewHandlers constructs the Handlers for the user domain.
func NewHandlers(service *userbus.Service) *Handlers {
	return &Handlers{service: service}
}

// Routes declares the routes served by this application.
func (h *Handlers) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /users", h.ListUsers)
}

// ListUsers handles a GET request and responds with all the users
// in the system.
func (h *Handlers) ListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.service.ListUsers(r.Context())
	if err != nil {
		slog.ErrorContext(r.Context(), "userapp: ListUsers", "error", err)
		api.WriteJSON(w, http.StatusInternalServerError, struct {
			Error string `json:"error"`
		}{Error: "failed to fetch users"})
		return
	}

	if err := api.WriteJSON(w, http.StatusOK, fromBusUsersResponse(users)); err != nil {
		slog.ErrorContext(r.Context(), "userapp: ListUsers", "error", err)
	}
}
