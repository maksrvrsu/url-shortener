package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"url-shortener/internal/http-server/handlers/redirect"
	deleteHandler "url-shortener/internal/http-server/handlers/url/delete"
	"url-shortener/internal/http-server/handlers/url/get"
	"url-shortener/internal/http-server/handlers/url/save"
	"url-shortener/internal/http-server/handlers/url/update"
	"url-shortener/internal/service"
	"url-shortener/internal/storage"
)

type inMemoryStorage struct {
	mu   sync.RWMutex
	data map[string]string
}

func newInMemoryStorage() *inMemoryStorage {
	return &inMemoryStorage{
		data: make(map[string]string),
	}
}

func (s *inMemoryStorage) SaveURL(ctx context.Context, urlToSave, alias string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.data[alias]; exists {
		return storage.ErrURLExists
	}
	s.data[alias] = urlToSave
	return nil
}

func (s *inMemoryStorage) GetURL(ctx context.Context, alias string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	u, exists := s.data[alias]
	if !exists {
		return "", storage.ErrURLNotFound
	}
	return u, nil
}

func (s *inMemoryStorage) UpdateURL(ctx context.Context, alias, newURL string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.data[alias]; !exists {
		return storage.ErrURLNotFound
	}
	s.data[alias] = newURL
	return nil
}

func (s *inMemoryStorage) DeleteURL(ctx context.Context, alias string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.data[alias]; !exists {
		return storage.ErrURLNotFound
	}
	delete(s.data, alias)
	return nil
}

type failingStorage struct{}

func (f *failingStorage) SaveURL(ctx context.Context, urlToSave, alias string) error {
	return errors.New("storage failure")
}

func (f *failingStorage) GetURL(ctx context.Context, alias string) (string, error) {
	return "", errors.New("storage failure")
}

func (f *failingStorage) UpdateURL(ctx context.Context, alias, newURL string) error {
	return errors.New("storage failure")
}

func (f *failingStorage) DeleteURL(ctx context.Context, alias string) error {
	return errors.New("storage failure")
}

func getCredentials(t *testing.T) (string, string) {
	t.Helper()
	_ = godotenv.Load("../.env")
	_ = godotenv.Load(".env")

	user := os.Getenv("HTTP_SERVER_USER")
	if user == "" {
		user = "test_user"
		t.Setenv("HTTP_SERVER_USER", user)
	}

	pass := os.Getenv("HTTP_SERVER_PASSWORD")
	if pass == "" {
		pass = "test_password"
		t.Setenv("HTTP_SERVER_PASSWORD", pass)
	}

	return user, pass
}

func setupTestApp(t *testing.T, st service.URLStorage) (*chi.Mux, string, string) {
	t.Helper()
	user, pass := getCredentials(t)

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := service.New(log, st, 6)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)

	r.Get("/{alias}", redirect.New(log, svc))
	r.Get("/url/{alias}", get.New(log, svc))

	r.Group(func(sub chi.Router) {
		sub.Use(middleware.BasicAuth("url-shortener", map[string]string{
			user: pass,
		}))

		sub.Post("/url", save.New(log, svc))
		sub.Put("/url/{alias}", update.New(log, svc))
		sub.Delete("/url/{alias}", deleteHandler.New(log, svc))
	})

	return r, user, pass
}

func TestSave_Success(t *testing.T) {
	st := newInMemoryStorage()
	router, user, pass := setupTestApp(t, st)

	body := []byte(`{"url": "https://google.com", "alias": "google-test"}`)
	req := httptest.NewRequest(http.MethodPost, "/url", bytes.NewReader(body))
	req.SetBasicAuth(user, pass)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)

	var res map[string]any
	err := json.NewDecoder(rec.Body).Decode(&res)
	require.NoError(t, err)
	assert.Equal(t, "OK", res["status"])
	assert.Equal(t, "google-test", res["alias"])
}

func TestSave_BadRequest(t *testing.T) {
	st := newInMemoryStorage()
	router, user, pass := setupTestApp(t, st)

	malformedJSON := []byte(`{"url": "https://google.com", alias: broken}`)
	req := httptest.NewRequest(http.MethodPost, "/url", bytes.NewReader(malformedJSON))
	req.SetBasicAuth(user, pass)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var res map[string]any
	err := json.NewDecoder(rec.Body).Decode(&res)
	require.NoError(t, err)
	assert.Equal(t, "Error", res["status"])
}

func TestSave_Conflict(t *testing.T) {
	st := newInMemoryStorage()
	router, user, pass := setupTestApp(t, st)

	body := []byte(`{"url": "https://google.com", "alias": "existing-alias"}`)
	req1 := httptest.NewRequest(http.MethodPost, "/url", bytes.NewReader(body))
	req1.SetBasicAuth(user, pass)
	req1.Header.Set("Content-Type", "application/json")
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)
	require.Equal(t, http.StatusCreated, rec1.Code)

	req2 := httptest.NewRequest(http.MethodPost, "/url", bytes.NewReader(body))
	req2.SetBasicAuth(user, pass)
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)

	assert.Equal(t, http.StatusConflict, rec2.Code)

	var res map[string]any
	err := json.NewDecoder(rec2.Body).Decode(&res)
	require.NoError(t, err)
	assert.Equal(t, "Error", res["status"])
	assert.Contains(t, res["error"], "alias already exists")
}

func TestSave_InternalError(t *testing.T) {
	st := &failingStorage{}
	router, user, pass := setupTestApp(t, st)

	body := []byte(`{"url": "https://google.com", "alias": "fail-save"}`)
	req := httptest.NewRequest(http.MethodPost, "/url", bytes.NewReader(body))
	req.SetBasicAuth(user, pass)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	var res map[string]any
	err := json.NewDecoder(rec.Body).Decode(&res)
	require.NoError(t, err)
	assert.Equal(t, "Error", res["status"])
	assert.Equal(t, "failed to add url", res["error"])
}

func TestRedirect_Success(t *testing.T) {
	st := newInMemoryStorage()
	router, user, pass := setupTestApp(t, st)

	body := []byte(`{"url": "https://github.com", "alias": "gh"}`)
	saveReq := httptest.NewRequest(http.MethodPost, "/url", bytes.NewReader(body))
	saveReq.SetBasicAuth(user, pass)
	saveReq.Header.Set("Content-Type", "application/json")
	saveRec := httptest.NewRecorder()
	router.ServeHTTP(saveRec, saveReq)
	require.Equal(t, http.StatusCreated, saveRec.Code)

	req := httptest.NewRequest(http.MethodGet, "/gh", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "https://github.com", rec.Header().Get("Location"))
}

func TestRedirect_NotFound(t *testing.T) {
	st := newInMemoryStorage()
	router, _, _ := setupTestApp(t, st)

	req := httptest.NewRequest(http.MethodGet, "/non-existent-random-alias", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)

	var res map[string]any
	err := json.NewDecoder(rec.Body).Decode(&res)
	require.NoError(t, err)
	assert.Equal(t, "Error", res["status"])
	assert.Equal(t, "not found", res["error"])
}

func TestRedirect_InternalError(t *testing.T) {
	st := &failingStorage{}
	router, _, _ := setupTestApp(t, st)

	req := httptest.NewRequest(http.MethodGet, "/fail-redir", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	var res map[string]any
	err := json.NewDecoder(rec.Body).Decode(&res)
	require.NoError(t, err)
	assert.Equal(t, "Error", res["status"])
	assert.Equal(t, "internal error", res["error"])
}

func TestGetURL_Success(t *testing.T) {
	st := newInMemoryStorage()
	router, user, pass := setupTestApp(t, st)

	body := []byte(`{"url": "https://golang.org", "alias": "go-lang"}`)
	saveReq := httptest.NewRequest(http.MethodPost, "/url", bytes.NewReader(body))
	saveReq.SetBasicAuth(user, pass)
	saveReq.Header.Set("Content-Type", "application/json")
	saveRec := httptest.NewRecorder()
	router.ServeHTTP(saveRec, saveReq)

	req := httptest.NewRequest(http.MethodGet, "/url/go-lang", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var res map[string]any
	err := json.NewDecoder(rec.Body).Decode(&res)
	require.NoError(t, err)
	assert.Equal(t, "OK", res["status"])
	assert.Equal(t, "https://golang.org", res["url"])
	assert.Equal(t, "go-lang", res["alias"])
}

func TestGetURL_NotFound(t *testing.T) {
	st := newInMemoryStorage()
	router, _, _ := setupTestApp(t, st)

	req := httptest.NewRequest(http.MethodGet, "/url/unknown-alias", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestGetURL_InternalError(t *testing.T) {
	st := &failingStorage{}
	router, _, _ := setupTestApp(t, st)

	req := httptest.NewRequest(http.MethodGet, "/url/fail-get", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	var res map[string]any
	err := json.NewDecoder(rec.Body).Decode(&res)
	require.NoError(t, err)
	assert.Equal(t, "Error", res["status"])
	assert.Equal(t, "internal error", res["error"])
}

func TestUpdate_Success(t *testing.T) {
	st := newInMemoryStorage()
	router, user, pass := setupTestApp(t, st)

	body := []byte(`{"url": "https://old-domain.com", "alias": "site"}`)
	saveReq := httptest.NewRequest(http.MethodPost, "/url", bytes.NewReader(body))
	saveReq.SetBasicAuth(user, pass)
	saveReq.Header.Set("Content-Type", "application/json")
	saveRec := httptest.NewRecorder()
	router.ServeHTTP(saveRec, saveReq)

	updateBody := []byte(`{"url": "https://new-domain.com"}`)
	updateReq := httptest.NewRequest(http.MethodPut, "/url/site", bytes.NewReader(updateBody))
	updateReq.SetBasicAuth(user, pass)
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	router.ServeHTTP(updateRec, updateReq)

	assert.Equal(t, http.StatusOK, updateRec.Code)

	redirReq := httptest.NewRequest(http.MethodGet, "/site", nil)
	redirRec := httptest.NewRecorder()
	router.ServeHTTP(redirRec, redirReq)
	assert.Equal(t, "https://new-domain.com", redirRec.Header().Get("Location"))
}

func TestUpdate_BadRequest(t *testing.T) {
	st := newInMemoryStorage()
	router, user, pass := setupTestApp(t, st)

	brokenJSON := []byte(`{"url": not_valid_json}`)
	req := httptest.NewRequest(http.MethodPut, "/url/site", bytes.NewReader(brokenJSON))
	req.SetBasicAuth(user, pass)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestUpdate_NotFound(t *testing.T) {
	st := newInMemoryStorage()
	router, user, pass := setupTestApp(t, st)

	body := []byte(`{"url": "https://some-url.com"}`)
	req := httptest.NewRequest(http.MethodPut, "/url/non-existent-site", bytes.NewReader(body))
	req.SetBasicAuth(user, pass)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestUpdate_InternalError(t *testing.T) {
	st := &failingStorage{}
	router, user, pass := setupTestApp(t, st)

	body := []byte(`{"url": "https://some-url.com"}`)
	req := httptest.NewRequest(http.MethodPut, "/url/fail-update", bytes.NewReader(body))
	req.SetBasicAuth(user, pass)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	var res map[string]any
	err := json.NewDecoder(rec.Body).Decode(&res)
	require.NoError(t, err)
	assert.Equal(t, "Error", res["status"])
	assert.Equal(t, "failed to update url", res["error"])
}

func TestDelete_Success(t *testing.T) {
	st := newInMemoryStorage()
	router, user, pass := setupTestApp(t, st)

	body := []byte(`{"url": "https://delete-me.com", "alias": "temp-alias"}`)
	saveReq := httptest.NewRequest(http.MethodPost, "/url", bytes.NewReader(body))
	saveReq.SetBasicAuth(user, pass)
	saveReq.Header.Set("Content-Type", "application/json")
	saveRec := httptest.NewRecorder()
	router.ServeHTTP(saveRec, saveReq)

	delReq := httptest.NewRequest(http.MethodDelete, "/url/temp-alias", nil)
	delReq.SetBasicAuth(user, pass)
	delRec := httptest.NewRecorder()
	router.ServeHTTP(delRec, delReq)

	assert.Equal(t, http.StatusOK, delRec.Code)

	redirReq := httptest.NewRequest(http.MethodGet, "/temp-alias", nil)
	redirRec := httptest.NewRecorder()
	router.ServeHTTP(redirRec, redirReq)
	assert.Equal(t, http.StatusNotFound, redirRec.Code)
}

func TestDelete_NotFound(t *testing.T) {
	st := newInMemoryStorage()
	router, user, pass := setupTestApp(t, st)

	delReq := httptest.NewRequest(http.MethodDelete, "/url/non-existent-delete", nil)
	delReq.SetBasicAuth(user, pass)
	delRec := httptest.NewRecorder()
	router.ServeHTTP(delRec, delReq)

	assert.Equal(t, http.StatusNotFound, delRec.Code)
}

func TestDelete_InternalError(t *testing.T) {
	st := &failingStorage{}
	router, user, pass := setupTestApp(t, st)

	delReq := httptest.NewRequest(http.MethodDelete, "/url/fail-delete", nil)
	delReq.SetBasicAuth(user, pass)
	delRec := httptest.NewRecorder()
	router.ServeHTTP(delRec, delReq)

	assert.Equal(t, http.StatusInternalServerError, delRec.Code)

	var res map[string]any
	err := json.NewDecoder(delRec.Body).Decode(&res)
	require.NoError(t, err)
	assert.Equal(t, "Error", res["status"])
	assert.Equal(t, "failed to delete url", res["error"])
}

func TestAuth_Unauthorized(t *testing.T) {
	st := newInMemoryStorage()
	router, _, _ := setupTestApp(t, st)

	body := []byte(`{"url": "https://google.com"}`)

	reqNoAuth := httptest.NewRequest(http.MethodPost, "/url", bytes.NewReader(body))
	reqNoAuth.Header.Set("Content-Type", "application/json")
	recNoAuth := httptest.NewRecorder()
	router.ServeHTTP(recNoAuth, reqNoAuth)

	assert.Equal(t, http.StatusUnauthorized, recNoAuth.Code)

	reqWrongAuth := httptest.NewRequest(http.MethodPost, "/url", bytes.NewReader(body))
	reqWrongAuth.SetBasicAuth("wrong_user", "wrong_password")
	reqWrongAuth.Header.Set("Content-Type", "application/json")
	recWrongAuth := httptest.NewRecorder()
	router.ServeHTTP(recWrongAuth, reqWrongAuth)

	assert.Equal(t, http.StatusUnauthorized, recWrongAuth.Code)
}
