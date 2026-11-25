package postgres

import (
	"strings"
	"testing"
)

func TestSanitizeDatabaseURL(t *testing.T) {
	clean, err := SanitizeDatabaseURL("postgresql://postgres:postgres@localhost:5432/dashboard?schema=public")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(clean, "schema=") {
		t.Fatalf("expected schema param to be stripped, got %s", clean)
	}
	if !strings.Contains(clean, "search_path=public") {
		t.Fatalf("expected search_path=public, got %s", clean)
	}

	clean2, err := SanitizeDatabaseURL("postgresql://postgres:postgres@localhost:5432/dashboard?sslmode=disable")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if clean2 != "postgresql://postgres:postgres@localhost:5432/dashboard?sslmode=disable" {
		t.Fatalf("unexpected URL modification: %s", clean2)
	}
}
