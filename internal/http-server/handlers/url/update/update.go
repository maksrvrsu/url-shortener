package update

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	"github.com/go-playground/validator/v10"

	"url-shortener/internal/lib/api/response"
	"url-shortener/internal/lib/logger/sl"
	"url-shortener/internal/service"
)

type Request struct {
	URL string `json:"url" validate:"required,url"`
}

type URLUpdater interface {
	UpdateURL(ctx context.Context, alias, newURL string) error
}

func New(log *slog.Logger, urlUpdater URLUpdater) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.url.update.New"

		opLog := log.With(
			slog.String("op", op),
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

		alias := chi.URLParam(r, "alias")
		if alias == "" {
			opLog.Info("alias is empty")
			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, response.Error("invalid request"))
			return
		}

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

		err = urlUpdater.UpdateURL(r.Context(), alias, req.URL)
		if err != nil {
			if errors.Is(err, service.ErrAliasNotFound) {
				opLog.Info("url not found for update", slog.String("alias", alias))
				render.Status(r, http.StatusNotFound)
				render.JSON(w, r, response.Error("not found"))
				return
			}

			if errors.Is(err, service.ErrInvalidURL) {
				opLog.Info("invalid url provided", slog.String("url", req.URL))
				render.Status(r, http.StatusBadRequest)
				render.JSON(w, r, response.Error("invalid url"))
				return
			}

			opLog.Error("failed to update url", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, response.Error("failed to update url"))
			return
		}

		opLog.Info("url updated successfully", slog.String("alias", alias))

		render.Status(r, http.StatusOK)
		render.JSON(w, r, response.OK())
	}
}
