package v1

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/hay-kot/httpkit/errchain"
	"github.com/hay-kot/httpkit/server"
	"github.com/rs/zerolog/log"
	"github.com/sysadminsmedia/homebox/backend/internal/core/services"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/validate"
	"go.opentelemetry.io/otel/attribute"
)

// aiSuggestionImageTypes are the image MIME types accepted by the AI
// suggestion endpoint.
var aiSuggestionImageTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
	"image/gif":  true,
}

// HandleEntityAISuggest godoc
//
//	@Summary		AI Entity Suggestion
//	@Description	Analyzes a photo of an item with the configured AI provider and
//					suggests entity details (name, description, quantity, tags, location).
//					Tag and location suggestions are restricted to the group's existing
//					tags and locations.
//	@Tags			Entities
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			file	formData	file	true	"Photo of the item"
//	@Success		200		{object}	services.EntityAISuggestion
//	@Failure		422		{object}	validate.ErrorResponse
//	@Failure		502		{object}	validate.ErrorResponse
//	@Failure		503		{object}	validate.ErrorResponse
//	@Router			/v1/entities/ai-suggest [POST]
//	@Security		Bearer
func (ctrl *V1Controller) HandleEntityAISuggest() errchain.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		spanCtx, span := startEntityCtrlSpan(r.Context(), "controller.V1.HandleEntityAISuggest")
		defer span.End()

		if !ctrl.config.AI.Enabled || ctrl.config.AI.APIKey == "" {
			return validate.NewRequestError(errors.New("AI photo recognition is not enabled"), http.StatusServiceUnavailable)
		}

		err := r.ParseMultipartForm(ctrl.maxUploadSize << 20)
		if err != nil {
			recordCtrlSpanError(span, err)
			log.Err(err).Msg("failed to parse multipart form")
			return multipartFormError(err)
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			switch {
			case errors.Is(err, http.ErrMissingFile):
				log.Debug().Msg("file for ai suggestion is missing")
				return server.JSON(w, http.StatusUnprocessableEntity, validate.NewFieldErrors().Append("file", "file is required"))
			default:
				recordCtrlSpanError(span, err)
				log.Err(err).Msg("failed to get file from form")
				return validate.NewRequestError(err, http.StatusInternalServerError)
			}
		}
		defer func() { _ = file.Close() }()

		mimeType := header.Header.Get("Content-Type")
		if !aiSuggestionImageTypes[mimeType] {
			err := fmt.Errorf("unsupported image type %q: must be one of image/jpeg, image/png, image/webp, image/gif", mimeType)
			return validate.NewRequestError(err, http.StatusUnprocessableEntity)
		}

		image, err := io.ReadAll(file)
		if err != nil {
			recordCtrlSpanError(span, err)
			log.Err(err).Msg("failed to read uploaded file")
			return validate.NewRequestError(err, http.StatusInternalServerError)
		}

		ctx := services.NewContext(spanCtx)
		span.SetAttributes(
			attribute.String("group.id", ctx.GID.String()),
			attribute.String("image.mime_type", mimeType),
			attribute.Int("image.size", len(image)),
		)

		suggestion, err := ctrl.svc.AI.SuggestFromPhoto(ctx, ctx.GID, image, mimeType)
		if err != nil {
			recordCtrlSpanError(span, err)
			if errors.Is(err, services.ErrAIDisabled) {
				return validate.NewRequestError(err, http.StatusServiceUnavailable)
			}
			log.Err(err).Msg("ai suggestion failed")
			return validate.NewRequestError(err, http.StatusBadGateway)
		}

		return server.JSON(w, http.StatusOK, suggestion)
	}
}
