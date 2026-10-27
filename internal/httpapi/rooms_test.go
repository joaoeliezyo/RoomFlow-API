package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/Eliezyo/RoomFlow-API/internal/rooms"
)

type memoryRooms struct {
	next  int64
	items map[int64]rooms.Room
	err   error
}

func newMemoryRooms() *memoryRooms { return &memoryRooms{items: make(map[int64]rooms.Room)} }
func (s *memoryRooms) Create(_ context.Context, name string, capacity int) (rooms.Room, error) {
	if s.err != nil {
		return rooms.Room{}, s.err
	}
	s.next++
	r := rooms.Room{ID: s.next, Name: name, Capacity: capacity}
	s.items[r.ID] = r
	return r, nil
}
func (s *memoryRooms) List(context.Context) ([]rooms.Room, error) {
	if s.err != nil {
		return nil, s.err
	}
	items := make([]rooms.Room, 0, len(s.items))
	for _, r := range s.items {
		items = append(items, r)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, nil
}
func (s *memoryRooms) Get(_ context.Context, id int64) (rooms.Room, error) {
	if s.err != nil {
		return rooms.Room{}, s.err
	}
	r, ok := s.items[id]
	if !ok {
		return rooms.Room{}, rooms.ErrNotFound
	}
	return r, nil
}
func (s *memoryRooms) Update(_ context.Context, id int64, u rooms.Update) (rooms.Room, error) {
	if s.err != nil {
		return rooms.Room{}, s.err
	}
	r, ok := s.items[id]
	if !ok {
		return rooms.Room{}, rooms.ErrNotFound
	}
	if u.Name != nil {
		r.Name = *u.Name
	}
	if u.Capacity != nil {
		r.Capacity = *u.Capacity
	}
	s.items[id] = r
	return r, nil
}
func (s *memoryRooms) Delete(_ context.Context, id int64) error {
	if s.err != nil {
		return s.err
	}
	if _, ok := s.items[id]; !ok {
		return rooms.ErrNotFound
	}
	delete(s.items, id)
	return nil
}

func requestRooms(t *testing.T, store *memoryRooms, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	NewHandler(fakeDB{}, store, nil).ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
	return w
}

func TestRoomLifecycle(t *testing.T) {
	store := newMemoryRooms()
	if got := requestRooms(t, store, "GET", "/rooms", ""); got.Code != 200 || strings.TrimSpace(got.Body.String()) != `{"data":[]}` {
		t.Fatalf("empty list: %d %s", got.Code, got.Body.String())
	}
	created := requestRooms(t, store, "POST", "/rooms", `{"name":"  Sala A  ","capacity":4}`)
	if created.Code != 201 || created.Header().Get("Location") != "/rooms/1" {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	var payload struct {
		Data rooms.Room `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &payload); err != nil || payload.Data.Name != "Sala A" {
		t.Fatalf("created = %+v, %v", payload, err)
	}
	if got := requestRooms(t, store, "GET", "/rooms", ""); got.Code != 200 || !strings.Contains(got.Body.String(), `"name":"Sala A"`) {
		t.Fatalf("filled list: %d %s", got.Code, got.Body.String())
	}
	if got := requestRooms(t, store, "GET", "/rooms/1", ""); got.Code != 200 {
		t.Fatalf("get: %d %s", got.Code, got.Body.String())
	}
	updated := requestRooms(t, store, "PATCH", "/rooms/1", `{"capacity":8}`)
	if err := json.Unmarshal(updated.Body.Bytes(), &payload); err != nil || updated.Code != 200 || payload.Data.Name != "Sala A" || payload.Data.Capacity != 8 {
		t.Fatalf("partial update: %d %s", updated.Code, updated.Body.String())
	}
	if got := requestRooms(t, store, "DELETE", "/rooms/1", ""); got.Code != 204 || got.Body.Len() != 0 {
		t.Fatalf("delete: %d %s", got.Code, got.Body.String())
	}
	if got := requestRooms(t, store, "GET", "/rooms/1", ""); got.Code != 404 {
		t.Fatalf("missing get: %d", got.Code)
	}
	if got := requestRooms(t, store, "DELETE", "/rooms/1", ""); got.Code != 404 {
		t.Fatalf("missing delete: %d", got.Code)
	}
}

func TestRoomValidation(t *testing.T) {
	store := newMemoryRooms()
	for _, tc := range []struct{ method, path, body string }{
		{"POST", "/rooms", `{"name":" ","capacity":2}`},
		{"POST", "/rooms", `{"name":"A","capacity":0}`},
		{"POST", "/rooms", `{"name":"A","capacity":2147483648}`},
		{"POST", "/rooms", `{"name":"A\u0000B","capacity":2}`},
		{"POST", "/rooms", `{"name":"A","capacity":2`},
		{"POST", "/rooms", `{"name":"A","capacity":2,"other":true}`},
		{"POST", "/rooms", `{"name":"A","name":"B","capacity":2}`},
		{"POST", "/rooms", `{"name":"A","capacity":2} {}`},
		{"PATCH", "/rooms/1", `{}`},
		{"PATCH", "/rooms/1", `{"name":null}`},
		{"PATCH", "/rooms/1", `{"capacity":0}`},
		{"PATCH", "/rooms/1", `{"capacity":2147483648}`},
		{"PATCH", "/rooms/1", `{"name":"A\u0000B"}`},
		{"PATCH", "/rooms/1", `{"other":true}`},
		{"GET", "/rooms/0", ""},
		{"GET", "/rooms/abc", ""},
	} {
		got := requestRooms(t, store, tc.method, tc.path, tc.body)
		if got.Code != 400 || !strings.Contains(got.Body.String(), `"error"`) {
			t.Errorf("%s %s %q: %d %s", tc.method, tc.path, tc.body, got.Code, got.Body.String())
		}
	}
	if len(store.items) != 0 {
		t.Fatal("invalid requests changed store")
	}
}

func TestRoomStoreErrorIsInternal(t *testing.T) {
	store := newMemoryRooms()
	store.err = errors.New("database details")
	got := requestRooms(t, store, "GET", "/rooms", "")
	if got.Code != 500 || strings.Contains(got.Body.String(), "database details") {
		t.Fatalf("internal error: %d %s", got.Code, got.Body.String())
	}
}

func TestRoomDeleteWithReservations(t *testing.T) {
	store := newMemoryRooms()
	store.err = rooms.ErrHasReservations
	got := requestRooms(t, store, "DELETE", "/rooms/1", "")
	if got.Code != 409 || !strings.Contains(got.Body.String(), `"room_has_reservations"`) {
		t.Fatalf("room with reservations: %d %s", got.Code, got.Body.String())
	}
}
