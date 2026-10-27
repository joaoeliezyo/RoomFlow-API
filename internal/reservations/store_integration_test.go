package reservations

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Eliezyo/RoomFlow-API/internal/rooms"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		if os.Getenv("REQUIRE_INTEGRATION_TESTS") == "1" {
			t.Fatal("TEST_DATABASE_URL is required in integration mode")
		}
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestStoreIntegration(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	roomStore := rooms.NewStore(tx)
	store := NewStore(tx)
	room1, err := roomStore.Create(ctx, "Reservation test 1", 4)
	if err != nil {
		t.Fatal(err)
	}
	room2, err := roomStore.Create(ctx, "Reservation test 2", 4)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	first, err := store.Create(ctx, room1.ID, start, end)
	if err != nil || first.ID < 1 || first.CancelledAt != nil {
		t.Fatalf("Create = %+v, %v", first, err)
	}
	if got, err := store.Get(ctx, first.ID); err != nil || got.ID != first.ID || !got.StartsAt.Equal(start) {
		t.Fatalf("Get = %+v, %v", got, err)
	}
	// A PostgreSQL constraint error aborts the current transaction until a savepoint is restored.
	if _, err := tx.Exec(ctx, "SAVEPOINT expected_error"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(ctx, room1.ID, start.Add(30*time.Minute), end.Add(30*time.Minute)); !errors.Is(err, ErrConflict) {
		t.Fatalf("overlap = %v", err)
	}
	if _, err := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT expected_error"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(ctx, 999999999, start, end); !errors.Is(err, ErrRoomNotFound) {
		t.Fatalf("missing room = %v", err)
	}
	if _, err := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT expected_error"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(ctx, room1.ID, end, start); err == nil {
		t.Fatal("expected database period check")
	}
	if _, err := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT expected_error"); err != nil {
		t.Fatal(err)
	}
	adjacent, err := store.Create(ctx, room1.ID, end, end.Add(time.Hour))
	if err != nil {
		t.Fatalf("adjacent interval: %v", err)
	}
	otherRoom, err := store.Create(ctx, room2.ID, start, end)
	if err != nil {
		t.Fatalf("same time, different room: %v", err)
	}
	listed, err := store.List(ctx)
	if err != nil || len(listed) < 3 {
		t.Fatalf("List = %d, %v", len(listed), err)
	}
	var ownIDs []int64
	for _, item := range listed {
		if item.RoomID == room1.ID || item.RoomID == room2.ID {
			ownIDs = append(ownIDs, item.ID)
		}
	}
	if len(ownIDs) != 3 || ownIDs[0] != first.ID || ownIDs[1] != otherRoom.ID || ownIDs[2] != adjacent.ID {
		t.Fatalf("List order for test rooms = %v", ownIDs)
	}
	if _, err := tx.Exec(ctx, "SAVEPOINT expected_error"); err != nil {
		t.Fatal(err)
	}
	if err := roomStore.Delete(ctx, room1.ID); !errors.Is(err, rooms.ErrHasReservations) {
		t.Fatalf("delete room with reservations = %v", err)
	}
	if _, err := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT expected_error"); err != nil {
		t.Fatal(err)
	}
	cancelled, err := store.Cancel(ctx, first.ID)
	if err != nil || cancelled.CancelledAt == nil {
		t.Fatalf("Cancel = %+v, %v", cancelled, err)
	}
	if _, err := store.Cancel(ctx, first.ID); !errors.Is(err, ErrAlreadyCancelled) {
		t.Fatalf("second cancel = %v", err)
	}
	loaded, err := store.Get(ctx, first.ID)
	if err != nil || loaded.CancelledAt == nil || !loaded.CancelledAt.Equal(*cancelled.CancelledAt) {
		t.Fatalf("cancel timestamp changed: %+v, %v", loaded, err)
	}
	if _, err := store.Create(ctx, room1.ID, start, end); err != nil {
		t.Fatalf("reuse cancelled interval: %v", err)
	}
	historyRoom, err := roomStore.Create(ctx, "Cancelled history room", 2)
	if err != nil {
		t.Fatal(err)
	}
	historyReservation, err := store.Create(ctx, historyRoom.ID, start, end)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Cancel(ctx, historyReservation.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "SAVEPOINT expected_error"); err != nil {
		t.Fatal(err)
	}
	if err := roomStore.Delete(ctx, historyRoom.ID); !errors.Is(err, rooms.ErrHasReservations) {
		t.Fatalf("delete room with cancelled reservation history = %v", err)
	}
	if _, err := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT expected_error"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, 999999999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing reservation = %v", err)
	}
	if _, err := store.Cancel(ctx, 999999999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cancel missing reservation = %v", err)
	}
}

func TestConcurrentOverlap(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	room, err := rooms.NewStore(pool).Create(ctx, "Concurrent reservations", 2)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM reservations WHERE room_id = $1`, room.ID); err != nil {
			t.Errorf("clean up concurrent reservations: %v", err)
		}
		if err := rooms.NewStore(pool).Delete(context.Background(), room.ID); err != nil {
			t.Errorf("clean up concurrent room: %v", err)
		}
	})
	conn1, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn1.Release()
	conn2, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn2.Release()
	start := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)
	begin := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, conn := range []*pgxpool.Conn{conn1, conn2} {
		wg.Add(1)
		go func(conn *pgxpool.Conn) {
			defer wg.Done()
			<-begin
			_, err := NewStore(conn).Create(ctx, room.ID, start, start.Add(time.Hour))
			results <- err
		}(conn)
	}
	close(begin)
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, ErrConflict):
			conflict++
		default:
			t.Fatalf("unexpected concurrent insert error: %v", err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("concurrent inserts: success=%d conflict=%d", success, conflict)
	}
	t.Logf("simultaneous inserts on separate connections: success=%d conflict=%d", success, conflict)
}

func TestConcurrentCancel(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	room, err := rooms.NewStore(pool).Create(ctx, "Concurrent cancellation", 2)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DELETE FROM reservations WHERE room_id = $1`, room.ID); err != nil {
			t.Errorf("clean up cancellation reservation: %v", err)
		}
		if err := rooms.NewStore(pool).Delete(context.Background(), room.ID); err != nil {
			t.Errorf("clean up cancellation room: %v", err)
		}
	})
	start := time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)
	created, err := NewStore(pool).Create(ctx, room.ID, start, start.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	conn1, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn1.Release()
	conn2, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn2.Release()
	type outcome struct {
		reservation Reservation
		err         error
	}
	begin := make(chan struct{})
	results := make(chan outcome, 2)
	var wg sync.WaitGroup
	for _, conn := range []*pgxpool.Conn{conn1, conn2} {
		wg.Add(1)
		go func(conn *pgxpool.Conn) {
			defer wg.Done()
			<-begin
			reservation, err := NewStore(conn).Cancel(ctx, created.ID)
			results <- outcome{reservation: reservation, err: err}
		}(conn)
	}
	close(begin)
	wg.Wait()
	close(results)
	success, alreadyCancelled := 0, 0
	var cancelledAt time.Time
	for result := range results {
		switch {
		case result.err == nil:
			success++
			if result.reservation.CancelledAt == nil {
				t.Fatal("successful cancellation has no timestamp")
			}
			cancelledAt = *result.reservation.CancelledAt
		case errors.Is(result.err, ErrAlreadyCancelled):
			alreadyCancelled++
		default:
			t.Fatalf("unexpected concurrent cancellation error: %v", result.err)
		}
	}
	if success != 1 || alreadyCancelled != 1 {
		t.Fatalf("concurrent cancellations: success=%d already_cancelled=%d", success, alreadyCancelled)
	}
	stored, err := NewStore(pool).Get(ctx, created.ID)
	if err != nil || stored.CancelledAt == nil || !stored.CancelledAt.Equal(cancelledAt) {
		t.Fatalf("stored cancellation timestamp = %+v, %v; expected %v", stored, err, cancelledAt)
	}
	t.Logf("simultaneous cancellations on separate connections: success=%d already_cancelled=%d", success, alreadyCancelled)
}
