package cache

import "testing"

func TestNewRedisStore_ParsesURL(t *testing.T) {
	store, err := NewRedisStore("redis://localhost:6379/0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("unexpected close error: %v", err)
	}

	if _, err := NewRedisStore("://invalid"); err == nil {
		t.Fatal("expected error for invalid redis URL")
	}
}
