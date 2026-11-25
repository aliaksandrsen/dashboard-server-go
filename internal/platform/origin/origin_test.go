package origin

import (
	"errors"
	"slices"
	"testing"
)

func TestGetAllowedOrigins(t *testing.T) {
	got := GetAllowedOrigins("")
	if !slices.Equal(got, DefaultAllowedOrigins) {
		t.Fatalf("expected default origins, got %v", got)
	}

	got = GetAllowedOrigins(" https://app.example.com/ , http://localhost:3001// ")
	want := []string{"https://app.example.com", "http://localhost:3001"}
	if !slices.Equal(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestResolveAllowedOrigin(t *testing.T) {
	allowed := []string{"http://localhost:5173", "https://dashboard.example.com"}

	got, err := ResolveAllowedOrigin("", allowed)
	if err != nil || got != "http://localhost:5173" {
		t.Fatalf("expected fallback origin, got %q, err=%v", got, err)
	}

	got, err = ResolveAllowedOrigin("https://dashboard.example.com/", allowed)
	if err != nil || got != "https://dashboard.example.com" {
		t.Fatalf("expected normalized match, got %q, err=%v", got, err)
	}

	_, err = ResolveAllowedOrigin("https://evil.com", allowed)
	if !errors.Is(err, ErrOriginNotAllowed) {
		t.Fatalf("expected ErrOriginNotAllowed, got %v", err)
	}
}
