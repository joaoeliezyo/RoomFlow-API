package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeDB struct{ err error }

func (db fakeDB) Ping(context.Context) error { return db.err }

func TestHealth(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
		body   string
	}{
		{"healthy", nil, http.StatusOK, `{"data":{"status":"ok"}}`},
		{"database unavailable", errors.New("secret database detail"), http.StatusServiceUnavailable, `{"error":{"code":"service_unavailable","message":"database unavailable"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
			response := httptest.NewRecorder()
			NewHandler(fakeDB{tc.err}, nil).ServeHTTP(response, request)
			if response.Code != tc.status || strings.TrimSpace(response.Body.String()) != tc.body {
				t.Fatalf("response = %d %s", response.Code, response.Body.String())
			}
			if got := response.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
				t.Fatalf("Content-Type = %q", got)
			}
		})
	}
}

func TestHTTPErrorResponses(t *testing.T) {
	for _, tc := range []struct {
		method string
		path   string
		status int
		body   string
	}{
		{http.MethodPost, "/healthz", http.StatusMethodNotAllowed, `{"error":{"code":"method_not_allowed","message":"method not allowed"}}`},
		{http.MethodGet, "/unknown", http.StatusNotFound, `{"error":{"code":"not_found","message":"resource not found"}}`},
	} {
		response := httptest.NewRecorder()
		NewHandler(fakeDB{}, nil).ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, nil))
		if response.Code != tc.status || strings.TrimSpace(response.Body.String()) != tc.body {
			t.Fatalf("%s %s: response = %d %s", tc.method, tc.path, response.Code, response.Body.String())
		}
	}
}
