package types

import "testing"

func TestNewUserName(t *testing.T) {
	t.Parallel()

	t.Run("valid name", func(t *testing.T) {
		t.Parallel()

		name, err := NewUserName("  Alice  ")
		if err != nil {
			t.Fatal(err)
		}

		if name != "Alice" {
			t.Errorf("name: got %q, want %q", name, "Alice")
		}
	})

	t.Run("empty name", func(t *testing.T) {
		t.Parallel()

		if _, err := NewUserName("   "); err == nil {
			t.Error("expected error for empty name")
		}
	})

	t.Run("name too long", func(t *testing.T) {
		t.Parallel()

		long := make([]byte, 0)
		for range maxUserNameLen + 1 {
			long = append(long, 'a')
		}

		if _, err := NewUserName(string(long)); err == nil {
			t.Error("expected error for long name")
		}
	})
}
