package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/Eliezyo/RoomFlow-API/internal/reservations"
)

type reservationStore interface {
	Create(context.Context, int64, time.Time, time.Time) (reservations.Reservation, error)
	List(context.Context) ([]reservations.Reservation, error)
	Get(context.Context, int64) (reservations.Reservation, error)
	Cancel(context.Context, int64) (reservations.Reservation, error)
}

func registerReservations(mux *http.ServeMux, store reservationStore) {
	mux.HandleFunc("/reservations", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			createReservation(w, r, store)
		case http.MethodGet:
			listReservations(w, r, store)
		default:
			methodNotAllowed(w, "GET, POST")
		}
	})
	mux.HandleFunc("/reservations/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := reservationID(w, r)
		if !ok {
			return
		}
		if r.Method != http.MethodGet {
			methodNotAllowed(w, "GET")
			return
		}
		reservation, err := store.Get(r.Context(), id)
		if err != nil {
			reservationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": reservation})
	})
	mux.HandleFunc("/reservations/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		id, ok := reservationID(w, r)
		if !ok {
			return
		}
		if r.Method != http.MethodPost {
			methodNotAllowed(w, "POST")
			return
		}
		reservation, err := store.Cancel(r.Context(), id)
		if err != nil {
			reservationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": reservation})
	})
}

func reservationID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, "invalid_id", "reservation id must be a positive integer")
		return 0, false
	}
	return id, true
}

func createReservation(w http.ResponseWriter, r *http.Request, store reservationStore) {
	var body struct {
		RoomID   json.RawMessage `json:"room_id"`
		StartsAt json.RawMessage `json:"starts_at"`
		EndsAt   json.RawMessage `json:"ends_at"`
	}
	if err := decodeBody(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid JSON body or unknown field")
		return
	}
	var roomID int64
	if len(body.RoomID) == 0 || json.Unmarshal(body.RoomID, &roomID) != nil || roomID < 1 {
		writeError(w, http.StatusBadRequest, "invalid_reservation", "room_id must be a positive integer")
		return
	}
	startsAt, err := parseTimestamp(body.StartsAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_reservation", "starts_at must be an RFC3339 timestamp with timezone")
		return
	}
	endsAt, err := parseTimestamp(body.EndsAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_reservation", "ends_at must be an RFC3339 timestamp with timezone")
		return
	}
	if !endsAt.After(startsAt) {
		writeError(w, http.StatusBadRequest, "invalid_reservation", "ends_at must be after starts_at")
		return
	}
	reservation, err := store.Create(r.Context(), roomID, startsAt, endsAt)
	if err != nil {
		reservationError(w, err)
		return
	}
	w.Header().Set("Location", "/reservations/"+strconv.FormatInt(reservation.ID, 10))
	writeJSON(w, http.StatusCreated, map[string]any{"data": reservation})
}

func parseTimestamp(raw json.RawMessage) (time.Time, error) {
	var value string
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return time.Time{}, errors.New("timestamp must be a string")
	}
	timestamp, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, err
	}
	return timestamp.UTC(), nil
}

func listReservations(w http.ResponseWriter, r *http.Request, store reservationStore) {
	items, err := store.List(r.Context())
	if err != nil {
		internalError(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func reservationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, reservations.ErrConflict):
		writeError(w, http.StatusConflict, "reservation_conflict", "reservation overlaps an active reservation")
	case errors.Is(err, reservations.ErrAlreadyCancelled):
		writeError(w, http.StatusConflict, "already_cancelled", "reservation already cancelled")
	case errors.Is(err, reservations.ErrRoomNotFound):
		writeError(w, http.StatusNotFound, "room_not_found", "room not found")
	case errors.Is(err, reservations.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "reservation not found")
	default:
		internalError(w)
	}
}
