package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Eliezyo/RoomFlow-API/internal/reservations"
	"github.com/Eliezyo/RoomFlow-API/internal/rooms"
	"github.com/jackc/pgx/v5/pgxpool"
)

type httpResult struct {
	status   int
	location string
	data     json.RawMessage
	code     string
	message  string
}

func httpCall(t *testing.T, client *http.Client, baseURL, method, path, body string, wantStatus int) httpResult {
	t.Helper()
	request, err := http.NewRequest(method, baseURL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != wantStatus {
		raw, _ := io.ReadAll(response.Body)
		t.Fatalf("%s %s: status=%d, body=%s", method, path, response.StatusCode, raw)
	}
	result := httpResult{status: response.StatusCode, location: response.Header.Get("Location")}
	if wantStatus == http.StatusNoContent {
		if raw, _ := io.ReadAll(response.Body); len(raw) != 0 {
			t.Fatalf("204 has body: %s", raw)
		}
		return result
	}
	if got := response.Header.Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	var envelope map[string]json.RawMessage
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatalf("invalid response JSON: %v", err)
	}
	if len(envelope) != 1 {
		t.Fatalf("unexpected response envelope: %v", envelope)
	}
	if wantStatus >= 400 {
		var failure map[string]string
		if err := json.Unmarshal(envelope["error"], &failure); err != nil || len(failure) != 2 || failure["code"] == "" || failure["message"] == "" {
			t.Fatalf("invalid error envelope: %v, %v", failure, err)
		}
		result.code, result.message = failure["code"], failure["message"]
		return result
	}
	if len(envelope["data"]) == 0 {
		t.Fatalf("missing data envelope: %v", envelope)
	}
	result.data = envelope["data"]
	return result
}

func TestHTTPIntegration(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		if os.Getenv("REQUIRE_INTEGRATION_TESTS") == "1" {
			t.Fatal("TEST_DATABASE_URL is required in integration mode")
		}
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL HTTP integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	server := httptest.NewServer(NewHandler(pool, rooms.NewStore(tx), reservations.NewStore(tx)))
	defer server.Close()
	client := server.Client()

	createdRoom := httpCall(t, client, server.URL, "POST", "/rooms", `{"name":"HTTP integration room","capacity":4}`, 201)
	var room rooms.Room
	if err := json.Unmarshal(createdRoom.data, &room); err != nil || room.ID < 1 || room.Name != "HTTP integration room" {
		t.Fatalf("room response = %+v, %v", room, err)
	}
	roomPath := fmt.Sprintf("/rooms/%d", room.ID)
	if createdRoom.location != roomPath {
		t.Fatalf("room Location = %q", createdRoom.location)
	}
	gotRoom := httpCall(t, client, server.URL, "GET", roomPath, "", 200)
	var fetched rooms.Room
	if err := json.Unmarshal(gotRoom.data, &fetched); err != nil || fetched != room {
		t.Fatalf("GET room = %+v, %v", fetched, err)
	}
	updatedRoom := httpCall(t, client, server.URL, "PATCH", roomPath, `{"capacity":6}`, 200)
	if err := json.Unmarshal(updatedRoom.data, &fetched); err != nil || fetched.Name != room.Name || fetched.Capacity != 6 {
		t.Fatalf("PATCH room = %+v, %v", fetched, err)
	}

	createBody := fmt.Sprintf(`{"room_id":%d,"starts_at":"2026-11-10T06:00:00-03:00","ends_at":"2026-11-10T10:00:00Z"}`, room.ID)
	createdReservation := httpCall(t, client, server.URL, "POST", "/reservations", createBody, 201)
	var reservation reservations.Reservation
	if err := json.Unmarshal(createdReservation.data, &reservation); err != nil || reservation.ID < 1 || reservation.RoomID != room.ID || reservation.CancelledAt != nil || reservation.StartsAt.Format(time.RFC3339) != "2026-11-10T09:00:00Z" {
		t.Fatalf("reservation response = %+v, %v", reservation, err)
	}
	reservationPath := fmt.Sprintf("/reservations/%d", reservation.ID)
	if createdReservation.location != reservationPath {
		t.Fatalf("reservation Location = %q", createdReservation.location)
	}

	if _, err := tx.Exec(ctx, "SAVEPOINT expected_conflict"); err != nil {
		t.Fatal(err)
	}
	overlap := fmt.Sprintf(`{"room_id":%d,"starts_at":"2026-11-10T09:30:00Z","ends_at":"2026-11-10T10:30:00Z"}`, room.ID)
	if got := httpCall(t, client, server.URL, "POST", "/reservations", overlap, 409); got.code != "reservation_conflict" {
		t.Fatalf("overlap code = %q", got.code)
	}
	if _, err := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT expected_conflict"); err != nil {
		t.Fatal(err)
	}
	adjacent := fmt.Sprintf(`{"room_id":%d,"starts_at":"2026-11-10T10:00:00Z","ends_at":"2026-11-10T11:00:00Z"}`, room.ID)
	httpCall(t, client, server.URL, "POST", "/reservations", adjacent, 201)
	var fetchedReservation reservations.Reservation
	if err := json.Unmarshal(httpCall(t, client, server.URL, "GET", reservationPath, "", 200).data, &fetchedReservation); err != nil || fetchedReservation.ID != reservation.ID {
		t.Fatalf("GET reservation = %+v, %v", fetchedReservation, err)
	}
	var listed []reservations.Reservation
	if err := json.Unmarshal(httpCall(t, client, server.URL, "GET", "/reservations", "", 200).data, &listed); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range listed {
		if item.ID == reservation.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("created reservation missing from list")
	}

	if _, err := tx.Exec(ctx, "SAVEPOINT expected_room_fk"); err != nil {
		t.Fatal(err)
	}
	if got := httpCall(t, client, server.URL, "DELETE", roomPath, "", 409); got.code != "room_has_reservations" {
		t.Fatalf("room deletion code = %q", got.code)
	}
	if _, err := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT expected_room_fk"); err != nil {
		t.Fatal(err)
	}
	cancelled := httpCall(t, client, server.URL, "POST", reservationPath+"/cancel", "", 200)
	if err := json.Unmarshal(cancelled.data, &fetchedReservation); err != nil || fetchedReservation.CancelledAt == nil {
		t.Fatalf("cancel response = %+v, %v", fetchedReservation, err)
	}
	if got := httpCall(t, client, server.URL, "POST", reservationPath+"/cancel", "", 409); got.code != "already_cancelled" {
		t.Fatalf("repeat cancel code = %q", got.code)
	}
	reused := fmt.Sprintf(`{"room_id":%d,"starts_at":"2026-11-10T09:00:00Z","ends_at":"2026-11-10T10:00:00Z"}`, room.ID)
	httpCall(t, client, server.URL, "POST", "/reservations", reused, 201)
	if got := httpCall(t, client, server.URL, "GET", reservationPath, "", 200); len(got.data) == 0 {
		t.Fatal("cancelled history missing")
	}

	// Hide rooms from this transaction to provoke a real PostgreSQL error without changing the schema.
	if _, err := tx.Exec(ctx, "SET LOCAL search_path TO roomflow_missing_schema"); err != nil {
		t.Fatal(err)
	}
	if got := httpCall(t, client, server.URL, "GET", "/rooms", "", 500); got.code != "internal_error" || got.message != "internal server error" {
		t.Fatalf("internal error response = %+v", got)
	}
}
