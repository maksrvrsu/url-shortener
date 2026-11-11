package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"url-shortener/internal/lib/logger/sl"
	"url-shortener/internal/lib/random"
	"url-shortener/internal/storage"
)

var (
	ErrAliasNotFound      = errors.New("alias not found")
	ErrAliasAlreadyExists = errors.New("alias already exists")
	ErrInvalidURL         = errors.New("invalid url")
	ErrInvalidAlias       = errors.New("invalid alias")
)

type URLStorage interface {
	SaveURL(ctx context.Context, urlToSave, alias string) error
	GetURL(ctx context.Context, alias string) (string, error)
	UpdateURL(ctx context.Context, alias, newURL string) error
	DeleteURL(ctx context.Context, alias string) error
}

type URLService struct {
	log         *slog.Logger
	storage     URLStorage
	aliasLength int
}

func New(log *slog.Logger, storage URLStorage, aliasLength int) *URLService {
	if aliasLength <= 0 {
		aliasLength = 6
	}
	return &URLService{
		log:         log,
		storage:     storage,
		aliasLength: aliasLength,
	}
}

func (s *URLService) SaveURL(ctx context.Context, rawURL, customAlias string) (string, error) {
	const op = "service.SaveURL"

	rawURL = strings.TrimSpace(rawURL)
	if err := validateURL(rawURL); err != nil {
		return "", ErrInvalidURL
	}

	customAlias = strings.TrimSpace(customAlias)

	if customAlias != "" {
		if strings.ContainsAny(customAlias, " /?#") {
			return "", ErrInvalidAlias
		}

		err := s.storage.SaveURL(ctx, rawURL, customAlias)
		if err != nil {
			if errors.Is(err, storage.ErrURLExists) {
				return "", ErrAliasAlreadyExists
			}
			s.log.Error("failed to save url with custom alias", sl.Err(err))
			return "", fmt.Errorf("%s: %w", op, err)
		}

		return customAlias, nil
	}

	const maxAttempts = 5
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		generatedAlias := random.NewRandomString(s.aliasLength)

		err := s.storage.SaveURL(ctx, rawURL, generatedAlias)
		if err == nil {
			return generatedAlias, nil
		}

		if errors.Is(err, storage.ErrURLExists) {
			s.log.Warn("alias collision occurred, retrying",
				slog.String("alias", generatedAlias),
				slog.Int("attempt", attempt),
			)
			continue
		}

		s.log.Error("failed to save url with generated alias", sl.Err(err))
		return "", fmt.Errorf("%s: %w", op, err)
	}

	return "", fmt.Errorf("%s: failed to generate unique alias after %d attempts", op, maxAttempts)
}

func (s *URLService) GetURL(ctx context.Context, alias string) (string, error) {
	const op = "service.GetURL"

	alias = strings.TrimSpace(alias)
	if alias == "" {
		return "", ErrAliasNotFound
	}

	resURL, err := s.storage.GetURL(ctx, alias)
	if err != nil {
		if errors.Is(err, storage.ErrURLNotFound) {
			return "", ErrAliasNotFound
		}
		s.log.Error("failed to get url", sl.Err(err))
		return "", fmt.Errorf("%s: %w", op, err)
	}

	return resURL, nil
}

func (s *URLService) UpdateURL(ctx context.Context, alias, newURL string) error {
	const op = "service.UpdateURL"

	alias = strings.TrimSpace(alias)
	if alias == "" {
		return ErrAliasNotFound
	}

	newURL = strings.TrimSpace(newURL)
	if err := validateURL(newURL); err != nil {
		return ErrInvalidURL
	}

	err := s.storage.UpdateURL(ctx, alias, newURL)
	if err != nil {
		if errors.Is(err, storage.ErrURLNotFound) {
			return ErrAliasNotFound
		}
		s.log.Error("failed to update url", sl.Err(err))
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (s *URLService) DeleteURL(ctx context.Context, alias string) error {
	const op = "service.DeleteURL"

	alias = strings.TrimSpace(alias)
	if alias == "" {
		return ErrAliasNotFound
	}

	err := s.storage.DeleteURL(ctx, alias)
	if err != nil {
		if errors.Is(err, storage.ErrURLNotFound) {
			return ErrAliasNotFound
		}
		s.log.Error("failed to delete url", sl.Err(err))
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func validateURL(rawURL string) error {
	if rawURL == "" {
		return ErrInvalidURL
	}

	u, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return ErrInvalidURL
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return ErrInvalidURL
	}

	if u.Host == "" {
		return ErrInvalidURL
	}

	return nil
}
