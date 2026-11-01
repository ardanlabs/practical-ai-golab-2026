package userapp

import (
	"context"
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"uuid"

	"github.com/ardanlabs/practical-ai-golab-2026/business/domain/userbus"
	"github.com/ardanlabs/practical-ai-golab-2026/business/types"
)

// fakeStore is a test double implementing userbus.Store.
type fakeStore struct {
	users []userbus.User
	err   error
}

func (f *fakeStore) FetchUsers(context.Context) ([]userbus.User, error) {
	return f.users, f.err
}

var (
	idAlice = uuid.MustParse("5b9f8f66-3f5e-4b57-9b3f-2f7b1a2c3d4e")
	idBob   = uuid.MustParse("7a1c2d3e-4f50-4a6b-8c9d-0e1f2a3b4c5d")
)

func TestListUsers(t *testing.T) {
	now := time.Now().UTC()

	store := &fakeStore{
		users: []userbus.User{
			{ID: idAlice, Name: "Alice", CreatedAt: now},
			{ID: idBob, Name: "Bob", CreatedAt: now.Add(time.Second)},
		},
	}

	handlers := NewHandlers(userbus.New(store))

	mux := http.NewServeMux()
	handlers.Routes(mux)

	srv := httptest.NewTestServer(t, mux)

	resp, err := srv.Client().Get("http://api.example.com/users")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var got listUsersResponse
	if err := json.UnmarshalRead(resp.Body, &got); err != nil {
		t.Fatal(err)
	}

	if len(got.Users) != 2 {
		t.Fatalf("users: got %d, want 2", len(got.Users))
	}

	want := []userResponse{
		{ID: idAlice, Name: "Alice", CreatedAt: now},
		{ID: idBob, Name: "Bob", CreatedAt: now.Add(time.Second)},
	}

	for i, w := range want {
		if got.Users[i].ID != w.ID || got.Users[i].Name != w.Name {
			t.Errorf("user[%d]: got %+v, want %+v", i, got.Users[i], w)
		}
	}
}

func TestListUsersStoreFailure(t *testing.T) {
	store := &fakeStore{err: context.DeadlineExceeded}

	handlers := NewHandlers(userbus.New(store))

	mux := http.NewServeMux()
	handlers.Routes(mux)

	srv := httptest.NewTestServer(t, mux)

	resp, err := srv.Client().Get("http://api.example.com/users")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
}

func TestFromBusUsersResponse(t *testing.T) {
	users := []userbus.User{
		{ID: idAlice, Name: types.UserName("Alice"), CreatedAt: time.Now().UTC()},
	}

	got := fromBusUsersResponse(users)

	if len(got.Users) != 1 {
		t.Fatalf("users: got %d, want 1", len(got.Users))
	}

	if got.Users[0].Name != "Alice" {
		t.Errorf("name: got %q, want %q", got.Users[0].Name, "Alice")
	}
}
