package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type databasePinger interface {
	Ping(context.Context) error
}

func NewHandler(db databasePinger, roomStore roomStore) http.Handler {
	mux := http.NewServeMux()
	registerRooms(mux, roomStore)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			writeError(w, http.StatusServiceUnavailable, "service_unavailable", "database unavailable")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "ok"}})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "resource not found")
	})
	return mux
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
