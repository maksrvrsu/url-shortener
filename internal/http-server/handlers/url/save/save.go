package save

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	"github.com/go-playground/validator/v10"

	"url-shortener/internal/lib/api/response"
	"url-shortener/internal/lib/logger/sl"
	"url-shortener/internal/service"
)

type Request struct {
	URL   string `json:"url" validate:"required,url"`
	Alias string `json:"alias,omitempty"`
}

type Response struct {
	response.Response
	Alias string `json:"alias,omitempty"`
}

type URLSaver interface {
	SaveURL(ctx context.Context, rawURL, alias string) (string, error)
}

func New(log *slog.Logger, urlSaver URLSaver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.url.save.New"

		opLog := log.With(
			slog.String("op", op),
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

		var req Request

		err := render.DecodeJSON(r.Body, &req)
		if errors.Is(err, io.EOF) {
			opLog.Error("request body is empty")
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, response.Error("empty request"))
			return
		}
		if err != nil {
			opLog.Error("failed to decode request body", sl.Err(err))
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, response.Error("failed to decode request"))
			return
		}

		opLog.Info("request body decoded", slog.Any("request", req))

		if err := validator.New().Struct(req); err != nil {
			var validateErr validator.ValidationErrors
			errors.As(err, &validateErr)

			opLog.Error("invalid request", sl.Err(err))
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, response.ValidationError(validateErr))
			return
		}

		alias, err := urlSaver.SaveURL(r.Context(), req.URL, req.Alias)
		if err != nil {
			if errors.Is(err, service.ErrAliasAlreadyExists) {
				opLog.Info("alias already exists", slog.String("alias", req.Alias))
				render.Status(r, http.StatusConflict)
				render.JSON(w, r, response.Error("alias already exists"))
				return
			}

			if errors.Is(err, service.ErrInvalidURL) || errors.Is(err, service.ErrInvalidAlias) {
				opLog.Info("invalid url or alias", sl.Err(err))
				render.Status(r, http.StatusBadRequest)
				render.JSON(w, r, response.Error(err.Error()))
				return
			}

			opLog.Error("failed to save url", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, response.Error("failed to add url"))
			return
		}

		opLog.Info("url added successfully", slog.String("alias", alias))

		render.Status(r, http.StatusCreated)
		render.JSON(w, r, Response{
			Response: response.OK(),
			Alias:    alias,
		})
	}
}
