package rooms

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestStoreIntegration(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		if os.Getenv("REQUIRE_INTEGRATION_TESTS") == "1" {
			t.Fatal("TEST_DATABASE_URL is required in integration mode")
		}
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	store := NewStore(tx)

	created, err := store.Create(ctx, "Integration Room", 3)
	if err != nil || created.ID < 1 {
		t.Fatalf("Create = %+v, %v", created, err)
	}
	listed, err := store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, room := range listed {
		if room.ID == created.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("new room missing from List")
	}
	loaded, err := store.Get(ctx, created.ID)
	if err != nil || loaded != created {
		t.Fatalf("Get = %+v, %v", loaded, err)
	}
	capacity := 7
	updated, err := store.Update(ctx, created.ID, Update{Capacity: &capacity})
	if err != nil || updated.Name != created.Name || updated.Capacity != 7 {
		t.Fatalf("Update = %+v, %v", updated, err)
	}
	if err := store.Delete(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get deleted room = %v", err)
	}
	if err := store.Delete(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete missing room = %v", err)
	}
}
