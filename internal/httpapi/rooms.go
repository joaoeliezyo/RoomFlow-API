package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/Eliezyo/RoomFlow-API/internal/rooms"
)

type roomStore interface {
	Create(context.Context, string, int) (rooms.Room, error)
	List(context.Context) ([]rooms.Room, error)
	Get(context.Context, int64) (rooms.Room, error)
	Update(context.Context, int64, rooms.Update) (rooms.Room, error)
	Delete(context.Context, int64) error
}

func registerRooms(mux *http.ServeMux, store roomStore) {
	mux.HandleFunc("/rooms", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			createRoom(w, r, store)
		case http.MethodGet:
			listRooms(w, r, store)
		default:
			methodNotAllowed(w, "GET, POST")
		}
	})
	mux.HandleFunc("/rooms/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id < 1 {
			writeError(w, http.StatusBadRequest, "invalid_id", "room id must be a positive integer")
			return
		}
		switch r.Method {
		case http.MethodGet:
			getRoom(w, r, store, id)
		case http.MethodPatch:
			updateRoom(w, r, store, id)
		case http.MethodDelete:
			deleteRoom(w, r, store, id)
		default:
			methodNotAllowed(w, "GET, PATCH, DELETE")
		}
	})
}

func methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
}

func decodeBody(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func createRoom(w http.ResponseWriter, r *http.Request, store roomStore) {
	var body struct {
		Name     string `json:"name"`
		Capacity int    `json:"capacity"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid JSON body or unknown field")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" || body.Capacity < 1 {
		writeError(w, http.StatusBadRequest, "invalid_room", "name is required and capacity must be positive")
		return
	}
	room, err := store.Create(r.Context(), body.Name, body.Capacity)
	if err != nil {
		internalError(w)
		return
	}
	w.Header().Set("Location", "/rooms/"+strconv.FormatInt(room.ID, 10))
	writeJSON(w, http.StatusCreated, map[string]any{"data": room})
}

func listRooms(w http.ResponseWriter, r *http.Request, store roomStore) {
	items, err := store.List(r.Context())
	if err != nil {
		internalError(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func getRoom(w http.ResponseWriter, r *http.Request, store roomStore, id int64) {
	room, err := store.Get(r.Context(), id)
	if err != nil {
		roomError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": room})
}

func updateRoom(w http.ResponseWriter, r *http.Request, store roomStore, id int64) {
	var body struct {
		Name     json.RawMessage `json:"name"`
		Capacity json.RawMessage `json:"capacity"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid JSON body or unknown field")
		return
	}
	if len(body.Name) == 0 && len(body.Capacity) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_room", "at least one field is required")
		return
	}
	var update rooms.Update
	if len(body.Name) > 0 {
		if bytes.Equal(body.Name, []byte("null")) {
			writeError(w, http.StatusBadRequest, "invalid_room", "name cannot be null")
			return
		}
		var name string
		if err := json.Unmarshal(body.Name, &name); err != nil || strings.TrimSpace(name) == "" {
			writeError(w, http.StatusBadRequest, "invalid_room", "name must be non-empty text")
			return
		}
		name = strings.TrimSpace(name)
		update.Name = &name
	}
	if len(body.Capacity) > 0 {
		if bytes.Equal(body.Capacity, []byte("null")) {
			writeError(w, http.StatusBadRequest, "invalid_room", "capacity cannot be null")
			return
		}
		var capacity int
		if err := json.Unmarshal(body.Capacity, &capacity); err != nil || capacity < 1 {
			writeError(w, http.StatusBadRequest, "invalid_room", "capacity must be a positive integer")
			return
		}
		update.Capacity = &capacity
	}
	room, err := store.Update(r.Context(), id, update)
	if err != nil {
		roomError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": room})
}

func deleteRoom(w http.ResponseWriter, r *http.Request, store roomStore, id int64) {
	if err := store.Delete(r.Context(), id); err != nil {
		roomError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func roomError(w http.ResponseWriter, err error) {
	if errors.Is(err, rooms.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "room not found")
		return
	}
	internalError(w)
}

func internalError(w http.ResponseWriter) {
	writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
}
