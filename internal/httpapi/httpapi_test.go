package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/clock"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/service"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/storage/sqlite"
)

type fakeHealth struct {
	pingErr error
	version int
	verErr  error
}

func (h fakeHealth) Ping(context.Context) error { return h.pingErr }
func (h fakeHealth) SchemaVersion(context.Context) (int, error) {
	return h.version, h.verErr
}

func testLogger(output io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestLiveAndReadyContracts(t *testing.T) {
	api := New(nil, fakeHealth{version: 7}, testLogger(io.Discard))
	server := httptest.NewServer(api.Handler())
	defer server.Close()
	for _, test := range []struct {
		path string
		key  string
		want any
	}{
		{path: "/livez", key: "status", want: "alive"},
		{path: "/readyz", key: "schema_version", want: float64(7)},
	} {
		t.Run(test.path, func(t *testing.T) {
			request, err := http.NewRequest(http.MethodGet, server.URL+test.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("X-Request-ID", "health-contract-request")
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusOK {
				t.Fatalf("status = %d", response.StatusCode)
			}
			if response.Header.Get("X-Request-ID") != "health-contract-request" || response.Header.Get("X-Content-Type-Options") != "nosniff" || response.Header.Get("Cache-Control") != "no-store" {
				t.Fatalf("response headers = %#v", response.Header)
			}
			var body map[string]any
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body[test.key] != test.want {
				t.Fatalf("body[%s] = %#v, want %#v", test.key, body[test.key], test.want)
			}
		})
	}
}

func TestReadinessMapsDependencyFailures(t *testing.T) {
	tests := []struct {
		name   string
		health fakeHealth
	}{
		{name: "ping", health: fakeHealth{pingErr: domain.WrapUnavailable("ping", errors.New("offline"))}},
		{name: "schema", health: fakeHealth{version: 1, verErr: domain.WrapUnavailable("schema", errors.New("locked"))}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			api := New(nil, test.health, testLogger(io.Discard))
			request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
			response := httptest.NewRecorder()
			api.Handler().ServeHTTP(response, request)
			if response.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
			}
			var body problem
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Error != "unavailable" || strings.Contains(body.Message, "offline") || strings.Contains(body.Message, "locked") {
				t.Fatalf("problem = %#v", body)
			}
		})
	}
}

func TestRequestIDAppearsInStructuredLog(t *testing.T) {
	var logs bytes.Buffer
	api := New(nil, fakeHealth{version: 1}, testLogger(&logs))
	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	request.Header.Set("X-Request-ID", "request-log-contract")
	response := httptest.NewRecorder()
	api.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &entry); err != nil {
		t.Fatalf("decode log %q: %v", logs.String(), err)
	}
	if entry["request_id"] != "request-log-contract" || entry["path"] != "/readyz" || entry["status"] != float64(http.StatusOK) {
		t.Fatalf("log entry = %#v", entry)
	}
}

func TestInvalidRequestIDIsReplaced(t *testing.T) {
	api := New(nil, fakeHealth{version: 1}, testLogger(io.Discard))
	for _, supplied := range []string{"", "short", strings.Repeat("x", 129)} {
		request := httptest.NewRequest(http.MethodGet, "/livez", nil)
		request.Header.Set("X-Request-ID", supplied)
		response := httptest.NewRecorder()
		api.Handler().ServeHTTP(response, request)
		got := response.Header().Get("X-Request-ID")
		if len(got) < 8 || got == supplied {
			t.Fatalf("supplied=%q generated=%q", supplied, got)
		}
	}
}

func newAuthenticatedAPI(t *testing.T) (*API, func()) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "http.db")
	database, err := sqlite.Open(context.Background(), "file:"+filepath.ToSlash(path)+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	manual := clock.NewManual(time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC))
	svc := service.New(database, manual, 12*time.Hour)
	if _, err := svc.EnsureBootstrapAdmin(context.Background(), "admin@example.com", "administrator-password"); err != nil {
		database.Close()
		t.Fatal(err)
	}
	return New(svc, database, testLogger(io.Discard)), func() { _ = database.Close() }
}

func performRequest(api *API, method, target, body, token string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	api.Handler().ServeHTTP(response, request)
	return response
}

func loginToken(t *testing.T, api *API) string {
	t.Helper()
	response := performRequest(api, http.MethodPost, "/v1/session", `{"email":"admin@example.com","password":"administrator-password"}`, "")
	if response.Code != http.StatusCreated {
		t.Fatalf("login status = %d, body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Token == "" {
		t.Fatalf("login body = %s, error=%v", response.Body.String(), err)
	}
	return body.Token
}

func TestSessionHTTPFlow(t *testing.T) {
	api, closeAPI := newAuthenticatedAPI(t)
	defer closeAPI()
	token := loginToken(t, api)
	me := performRequest(api, http.MethodGet, "/v1/me", "", token)
	if me.Code != http.StatusOK {
		t.Fatalf("me status = %d, body=%s", me.Code, me.Body.String())
	}
	var user userResponse
	if err := json.Unmarshal(me.Body.Bytes(), &user); err != nil {
		t.Fatal(err)
	}
	if user.Email != "admin@example.com" || user.Role != domain.RoleAdmin || !user.Active {
		t.Fatalf("me body = %#v", user)
	}
	logout := performRequest(api, http.MethodDelete, "/v1/session", "", token)
	if logout.Code != http.StatusNoContent || logout.Body.Len() != 0 {
		t.Fatalf("logout = %d %q", logout.Code, logout.Body.String())
	}
	revoked := performRequest(api, http.MethodGet, "/v1/me", "", token)
	if revoked.Code != http.StatusUnauthorized {
		t.Fatalf("revoked token status = %d, body=%s", revoked.Code, revoked.Body.String())
	}
}

func TestAuthenticationRejectsMissingAndMalformedBearerHeaders(t *testing.T) {
	api, closeAPI := newAuthenticatedAPI(t)
	defer closeAPI()
	for _, header := range []string{"", "Basic abc", "Bearer", "Bearer ", "Token value"} {
		request := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
		if header != "" {
			request.Header.Set("Authorization", header)
		}
		response := httptest.NewRecorder()
		api.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("header %q status = %d", header, response.Code)
		}
	}
}

func TestLoginJSONValidation(t *testing.T) {
	api, closeAPI := newAuthenticatedAPI(t)
	defer closeAPI()
	tests := []struct {
		name string
		body string
	}{
		{name: "malformed", body: `{"email":`},
		{name: "unknown field", body: `{"email":"admin@example.com","password":"administrator-password","extra":true}`},
		{name: "multiple values", body: `{"email":"admin@example.com","password":"administrator-password"} {}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := performRequest(api, http.MethodPost, "/v1/session", test.body, "")
			if response.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
			}
			var body problem
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Error != "validation_failed" {
				t.Fatalf("problem = %#v, error=%v", body, err)
			}
		})
	}
}

func TestInvalidPathAndQueryParametersUseValidationProblem(t *testing.T) {
	api, closeAPI := newAuthenticatedAPI(t)
	defer closeAPI()
	token := loginToken(t, api)
	tests := []string{
		"/v1/cases/x",
		"/v1/cases?page=0",
		"/v1/cases?page_size=201",
		"/v1/cases?submitted_after=not-a-time",
		"/v1/cases?forest_site_id=x",
	}
	for _, target := range tests {
		t.Run(target, func(t *testing.T) {
			response := performRequest(api, http.MethodGet, target, "", token)
			if response.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestWriteErrorStatusMapping(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
		code string
	}{
		{name: "validation", err: domain.FieldError{Field: "lux", Message: "invalid"}, want: 422, code: "validation_failed"},
		{name: "unauthenticated", err: domain.ErrUnauthenticated, want: 401, code: "unauthenticated"},
		{name: "forbidden", err: domain.ErrForbidden, want: 403, code: "forbidden"},
		{name: "not found", err: domain.ErrNotFound, want: 404, code: "not_found"},
		{name: "conflict", err: domain.ErrConflict, want: 409, code: "conflict"},
		{name: "transition", err: domain.ErrInvalidTransition, want: 409, code: "conflict"},
		{name: "lease", err: domain.ErrLeaseLost, want: 409, code: "conflict"},
		{name: "unavailable", err: domain.ErrUnavailable, want: 503, code: "unavailable"},
		{name: "cancelled", err: context.Canceled, want: 499, code: "request_cancelled"},
		{name: "internal", err: errors.New("sensitive database error"), want: 500, code: "internal_error"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			writeError(response, test.err)
			if response.Code != test.want {
				t.Fatalf("status = %d, want %d", response.Code, test.want)
			}
			var body problem
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Error != test.code {
				t.Fatalf("problem = %#v, error=%v", body, err)
			}
			if test.want == 500 && strings.Contains(body.Message, "sensitive") {
				t.Fatalf("internal detail leaked: %#v", body)
			}
		})
	}
}

func TestPanicRecoveryReturnsJSONAndKeepsServerUsable(t *testing.T) {
	var logs bytes.Buffer
	api := New(nil, fakeHealth{version: 1}, testLogger(&logs))
	api.mux.HandleFunc("GET /panic-test", func(http.ResponseWriter, *http.Request) { panic("test panic") })
	panicResponse := performRequest(api, http.MethodGet, "/panic-test", "", "")
	if panicResponse.Code != http.StatusInternalServerError || panicResponse.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("panic response = %d %#v %s", panicResponse.Code, panicResponse.Header(), panicResponse.Body.String())
	}
	healthy := performRequest(api, http.MethodGet, "/livez", "", "")
	if healthy.Code != http.StatusOK {
		t.Fatalf("server did not recover: %d %s", healthy.Code, healthy.Body.String())
	}
	if !strings.Contains(logs.String(), `"status":500`) || !strings.Contains(logs.String(), `"msg":"request panic"`) {
		t.Fatalf("panic request was not fully logged: %s", logs.String())
	}
}
