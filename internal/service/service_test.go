package service_test

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"url-shortener/internal/service"
	"url-shortener/internal/storage"
)

type mockStorage struct {
	mu           sync.Mutex
	data         map[string]string
	collideCount int
}

func newMockStorage() *mockStorage {
	return &mockStorage{
		data: make(map[string]string),
	}
}

func (m *mockStorage) SaveURL(ctx context.Context, urlToSave, alias string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.collideCount > 0 {
		m.collideCount--
		return storage.ErrURLExists
	}

	if _, exists := m.data[alias]; exists {
		return storage.ErrURLExists
	}

	m.data[alias] = urlToSave
	return nil
}

func (m *mockStorage) GetURL(ctx context.Context, alias string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	u, exists := m.data[alias]
	if !exists {
		return "", storage.ErrURLNotFound
	}
	return u, nil
}

func (m *mockStorage) UpdateURL(ctx context.Context, alias, newURL string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.data[alias]; !exists {
		return storage.ErrURLNotFound
	}
	m.data[alias] = newURL
	return nil
}

func (m *mockStorage) DeleteURL(ctx context.Context, alias string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.data[alias]; !exists {
		return storage.ErrURLNotFound
	}
	delete(m.data, alias)
	return nil
}

func dummyLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestService_SaveURL_SuccessCustom(t *testing.T) {
	st := newMockStorage()
	svc := service.New(dummyLogger(), st, 6)

	alias, err := svc.SaveURL(context.Background(), "https://google.com", "my-alias")
	require.NoError(t, err)
	assert.Equal(t, "my-alias", alias)

	gotURL, err := svc.GetURL(context.Background(), "my-alias")
	require.NoError(t, err)
	assert.Equal(t, "https://google.com", gotURL)
}

func TestService_SaveURL_ConflictCustom(t *testing.T) {
	st := newMockStorage()
	svc := service.New(dummyLogger(), st, 6)

	_, err := svc.SaveURL(context.Background(), "https://google.com", "my-alias")
	require.NoError(t, err)

	_, err = svc.SaveURL(context.Background(), "https://yandex.ru", "my-alias")
	require.ErrorIs(t, err, service.ErrAliasAlreadyExists)
}

func TestService_SaveURL_AutoGenerate(t *testing.T) {
	st := newMockStorage()
	svc := service.New(dummyLogger(), st, 6)

	alias, err := svc.SaveURL(context.Background(), "https://golang.org", "")
	require.NoError(t, err)
	assert.Len(t, alias, 6)

	gotURL, err := svc.GetURL(context.Background(), alias)
	require.NoError(t, err)
	assert.Equal(t, "https://golang.org", gotURL)
}

func TestService_SaveURL_CollisionRetry(t *testing.T) {
	st := newMockStorage()
	st.collideCount = 2
	svc := service.New(dummyLogger(), st, 6)

	alias, err := svc.SaveURL(context.Background(), "https://github.com", "")
	require.NoError(t, err)
	assert.NotEmpty(t, alias)
}

func TestService_SaveURL_InvalidURL(t *testing.T) {
	st := newMockStorage()
	svc := service.New(dummyLogger(), st, 6)

	_, err := svc.SaveURL(context.Background(), "ftp://not-http.com", "alias1")
	require.ErrorIs(t, err, service.ErrInvalidURL)

	_, err = svc.SaveURL(context.Background(), "not-a-valid-url", "alias2")
	require.ErrorIs(t, err, service.ErrInvalidURL)
}

func TestService_GetURL_NotFound(t *testing.T) {
	st := newMockStorage()
	svc := service.New(dummyLogger(), st, 6)

	_, err := svc.GetURL(context.Background(), "non-existent")
	require.ErrorIs(t, err, service.ErrAliasNotFound)
}

func TestService_UpdateURL_Success(t *testing.T) {
	st := newMockStorage()
	svc := service.New(dummyLogger(), st, 6)

	_, err := svc.SaveURL(context.Background(), "https://google.com", "alias-update")
	require.NoError(t, err)

	err = svc.UpdateURL(context.Background(), "alias-update", "https://duckduckgo.com")
	require.NoError(t, err)

	got, err := svc.GetURL(context.Background(), "alias-update")
	require.NoError(t, err)
	assert.Equal(t, "https://duckduckgo.com", got)
}

func TestService_UpdateURL_NotFound(t *testing.T) {
	st := newMockStorage()
	svc := service.New(dummyLogger(), st, 6)

	err := svc.UpdateURL(context.Background(), "non-existent", "https://duckduckgo.com")
	require.ErrorIs(t, err, service.ErrAliasNotFound)
}

func TestService_DeleteURL_Success(t *testing.T) {
	st := newMockStorage()
	svc := service.New(dummyLogger(), st, 6)

	_, err := svc.SaveURL(context.Background(), "https://google.com", "to-delete")
	require.NoError(t, err)

	err = svc.DeleteURL(context.Background(), "to-delete")
	require.NoError(t, err)

	_, err = svc.GetURL(context.Background(), "to-delete")
	require.ErrorIs(t, err, service.ErrAliasNotFound)
}

func TestService_DeleteURL_NotFound(t *testing.T) {
	st := newMockStorage()
	svc := service.New(dummyLogger(), st, 6)

	err := svc.DeleteURL(context.Background(), "non-existent")
	require.ErrorIs(t, err, service.ErrAliasNotFound)
}
