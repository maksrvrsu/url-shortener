package delete

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"

	"url-shortener/internal/lib/api/response"
	"url-shortener/internal/lib/logger/sl"
	"url-shortener/internal/service"
)

type URLDeleter interface {
	DeleteURL(ctx context.Context, alias string) error
}

func New(log *slog.Logger, urlDeleter URLDeleter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.url.delete.New"

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

		err := urlDeleter.DeleteURL(r.Context(), alias)
		if err != nil {
			if errors.Is(err, service.ErrAliasNotFound) {
				opLog.Info("url not found for deletion", slog.String("alias", alias))
				render.Status(r, http.StatusNotFound)
				render.JSON(w, r, response.Error("not found"))
				return
			}

			opLog.Error("failed to delete url", sl.Err(err))
			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, response.Error("failed to delete url"))
			return
		}

		opLog.Info("url deleted successfully", slog.String("alias", alias))

		render.Status(r, http.StatusOK)
		render.JSON(w, r, response.OK())
	}
}
