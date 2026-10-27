package httpapi

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Eliezyo/RoomFlow-API/internal/reservations"
)

type fakeReservations struct {
	items   map[int64]reservations.Reservation
	err     error
	created int
}

func newFakeReservations() *fakeReservations {
	return &fakeReservations{items: make(map[int64]reservations.Reservation)}
}
func (s *fakeReservations) Create(_ context.Context, roomID int64, startsAt, endsAt time.Time) (reservations.Reservation, error) {
	if s.err != nil {
		return reservations.Reservation{}, s.err
	}
	s.created++
	r := reservations.Reservation{ID: int64(s.created), RoomID: roomID, StartsAt: startsAt, EndsAt: endsAt}
	s.items[r.ID] = r
	return r, nil
}
func (s *fakeReservations) List(context.Context) ([]reservations.Reservation, error) {
	if s.err != nil {
		return nil, s.err
	}
	items := make([]reservations.Reservation, 0, len(s.items))
	for _, r := range s.items {
		items = append(items, r)
	}
	return items, nil
}
func (s *fakeReservations) Get(_ context.Context, id int64) (reservations.Reservation, error) {
	if s.err != nil {
		return reservations.Reservation{}, s.err
	}
	r, ok := s.items[id]
	if !ok {
		return reservations.Reservation{}, reservations.ErrNotFound
	}
	return r, nil
}
func (s *fakeReservations) Cancel(_ context.Context, id int64) (reservations.Reservation, error) {
	if s.err != nil {
		return reservations.Reservation{}, s.err
	}
	r, ok := s.items[id]
	if !ok {
		return reservations.Reservation{}, reservations.ErrNotFound
	}
	if r.CancelledAt != nil {
		return reservations.Reservation{}, reservations.ErrAlreadyCancelled
	}
	now := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	r.CancelledAt = &now
	s.items[id] = r
	return r, nil
}

func reservationRequest(t *testing.T, store *fakeReservations, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	NewHandler(fakeDB{}, nil, store).ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
	return w
}

func TestReservationHTTPFlow(t *testing.T) {
	store := newFakeReservations()
	if got := reservationRequest(t, store, "GET", "/reservations", ""); got.Code != 200 || strings.TrimSpace(got.Body.String()) != `{"data":[]}` {
		t.Fatalf("empty list: %d %s", got.Code, got.Body.String())
	}
	created := reservationRequest(t, store, "POST", "/reservations", `{"room_id":1,"starts_at":"2026-11-10T06:00:00-03:00","ends_at":"2026-11-10T10:00:00Z"}`)
	if created.Code != 201 || created.Header().Get("Location") != "/reservations/1" || !strings.Contains(created.Body.String(), `"starts_at":"2026-11-10T09:00:00Z"`) {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	if got := reservationRequest(t, store, "GET", "/reservations", ""); got.Code != 200 || !strings.Contains(got.Body.String(), `"id":1`) {
		t.Fatalf("list: %d %s", got.Code, got.Body.String())
	}
	if got := reservationRequest(t, store, "GET", "/reservations/1", ""); got.Code != 200 {
		t.Fatalf("get: %d %s", got.Code, got.Body.String())
	}
	if got := reservationRequest(t, store, "POST", "/reservations/1/cancel", ""); got.Code != 200 || !strings.Contains(got.Body.String(), `"cancelled_at":"2026-11-01T00:00:00Z"`) {
		t.Fatalf("cancel: %d %s", got.Code, got.Body.String())
	}
	if got := reservationRequest(t, store, "POST", "/reservations/1/cancel", ""); got.Code != 409 || !strings.Contains(got.Body.String(), `"already_cancelled"`) {
		t.Fatalf("repeat cancel: %d %s", got.Code, got.Body.String())
	}
	if got := reservationRequest(t, store, "GET", "/reservations/99", ""); got.Code != 404 {
		t.Fatalf("missing get: %d", got.Code)
	}
	if got := reservationRequest(t, store, "POST", "/reservations/99/cancel", ""); got.Code != 404 {
		t.Fatalf("missing cancel: %d", got.Code)
	}
}

func TestReservationInvalidRequests(t *testing.T) {
	store := newFakeReservations()
	valid := `{"room_id":1,"starts_at":"2026-11-10T09:00:00Z","ends_at":"2026-11-10T10:00:00Z"}`
	for _, body := range []string{
		``, `null`, `{}`, `{"room_id":null,"starts_at":"2026-11-10T09:00:00Z","ends_at":"2026-11-10T10:00:00Z"}`,
		`{"room_id":0,"starts_at":"2026-11-10T09:00:00Z","ends_at":"2026-11-10T10:00:00Z"}`,
		`{"room_id":1.5,"starts_at":"2026-11-10T09:00:00Z","ends_at":"2026-11-10T10:00:00Z"}`,
		`{"room_id":1,"starts_at":null,"ends_at":"2026-11-10T10:00:00Z"}`,
		`{"room_id":1,"starts_at":"2026-11-10T09:00:00","ends_at":"2026-11-10T10:00:00Z"}`,
		`{"room_id":1,"starts_at":"bad","ends_at":"2026-11-10T10:00:00Z"}`,
		`{"room_id":1,"starts_at":"2026-11-10T10:00:00Z","ends_at":"2026-11-10T09:00:00Z"}`,
		`{"room_id":1,"starts_at":"2026-11-10T09:00:00Z","ends_at":"2026-11-10T09:00:00Z"}`,
		`{"room_id":1,"starts_at":"2026-11-10T09:00:00Z","ends_at":"2026-11-10T10:00:00Z","extra":1}`,
		valid + valid,
	} {
		got := reservationRequest(t, store, "POST", "/reservations", body)
		if got.Code != 400 {
			t.Errorf("body %q: %d %s", body, got.Code, got.Body.String())
		}
	}
	if store.created != 0 {
		t.Fatal("invalid requests reached store")
	}
	for _, path := range []string{"/reservations/0", "/reservations/nope", "/reservations/9223372036854775808/cancel"} {
		if got := reservationRequest(t, store, "GET", path, ""); got.Code != 400 {
			t.Errorf("invalid id %s: %d", path, got.Code)
		}
	}
}

func TestReservationErrorMapping(t *testing.T) {
	valid := `{"room_id":1,"starts_at":"2026-11-10T09:00:00Z","ends_at":"2026-11-10T10:00:00Z"}`
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{reservations.ErrConflict, 409, "reservation_conflict"},
		{reservations.ErrRoomNotFound, 404, "room_not_found"},
		{errors.New("database secret"), 500, "internal_error"},
	} {
		store := newFakeReservations()
		store.err = tc.err
		got := reservationRequest(t, store, "POST", "/reservations", valid)
		if got.Code != tc.status || !strings.Contains(got.Body.String(), `"`+tc.code+`"`) || strings.Contains(got.Body.String(), "database secret") {
			t.Errorf("error %v: %d %s", tc.err, got.Code, got.Body.String())
		}
	}
}
