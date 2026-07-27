package v1

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/hay-kot/httpkit/errchain"
	"github.com/hay-kot/httpkit/server"
	"github.com/rs/zerolog/log"
	"github.com/sysadminsmedia/homebox/backend/internal/core/services"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/validate"
	"go.opentelemetry.io/otel/attribute"
)

// assistantVoiceResponse is the payload returned by HandleAssistantVoice: the
// speech-to-text transcript, the assistant's natural-language reply, and the
// action proposals the frontend should render as confirmation cards.
type assistantVoiceResponse struct {
	Transcript string                     `json:"transcript"`
	Reply      string                     `json:"reply"`
	Actions    []services.AssistantAction `json:"actions"`
}

// HandleAssistantVoice godoc
//
//	@Summary		AI Voice Assistant
//	@Description	Transcribes a recorded voice command with the group's configured
//					speech-to-text endpoint, interprets it with the configured AI
//					provider, and returns the transcript plus the assistant's reply and
//					action proposals (create location/item, query item/location).
//					Actions are never executed server-side; the client confirms them and
//					calls the existing APIs itself. Requires the assistant namespace in
//					the group settings to be enabled with a complete STT configuration.
//	@Tags			Assistant
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			audio	formData	file	true	"Recorded audio clip (max 10MB)"
//	@Param			history	formData	string	false	"JSON array of prior conversation messages [{\"role\":\"user|assistant\",\"content\":string}]"
//	@Success		200		{object}	assistantVoiceResponse
//	@Failure		400		{object}	validate.ErrorResponse
//	@Failure		409		{object}	validate.ErrorResponse
//	@Failure		413		{object}	validate.ErrorResponse
//	@Failure		422		{object}	validate.ErrorResponse
//	@Failure		502		{object}	validate.ErrorResponse
//	@Failure		503		{object}	validate.ErrorResponse
//	@Router			/v1/assistant/voice [POST]
//	@Security		Bearer
func (ctrl *V1Controller) HandleAssistantVoice() errchain.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		spanCtx, span := startEntityCtrlSpan(r.Context(), "controller.V1.HandleAssistantVoice")
		defer span.End()

		if !ctrl.config.AI.Enabled || ctrl.config.AI.APIKey == "" {
			return validate.NewRequestError(errors.New("AI is not enabled"), http.StatusServiceUnavailable)
		}

		err := r.ParseMultipartForm(services.AssistantMaxAudioBytes)
		if err != nil {
			recordCtrlSpanError(span, err)
			log.Err(err).Msg("failed to parse multipart form")
			return multipartFormError(err)
		}

		file, header, err := r.FormFile("audio")
		if err != nil {
			switch {
			case errors.Is(err, http.ErrMissingFile):
				log.Debug().Msg("audio for assistant voice request is missing")
				return server.JSON(w, http.StatusUnprocessableEntity, validate.NewFieldErrors().Append("audio", "audio is required"))
			default:
				recordCtrlSpanError(span, err)
				log.Err(err).Msg("failed to get audio from form")
				return validate.NewRequestError(err, http.StatusInternalServerError)
			}
		}
		defer func() { _ = file.Close() }()

		audio, err := io.ReadAll(io.LimitReader(file, services.AssistantMaxAudioBytes+1))
		if err != nil {
			recordCtrlSpanError(span, err)
			log.Err(err).Msg("failed to read uploaded audio")
			return validate.NewRequestError(err, http.StatusInternalServerError)
		}
		if len(audio) > services.AssistantMaxAudioBytes {
			return server.JSON(w, http.StatusUnprocessableEntity, validate.NewFieldErrors().Append("audio", "audio exceeds the 10MB size limit"))
		}
		if len(audio) == 0 {
			return server.JSON(w, http.StatusUnprocessableEntity, validate.NewFieldErrors().Append("audio", "audio is empty"))
		}

		var history []services.AssistantMessage
		if raw := strings.TrimSpace(r.FormValue("history")); raw != "" {
			if err := json.Unmarshal([]byte(raw), &history); err != nil {
				return server.JSON(w, http.StatusUnprocessableEntity, validate.NewFieldErrors().Append("history", "history must be a JSON array of messages"))
			}
		}

		mimeType := header.Header.Get("Content-Type")

		ctx := services.NewContext(spanCtx)
		span.SetAttributes(
			attribute.String("group.id", ctx.GID.String()),
			attribute.String("audio.mime_type", mimeType),
			attribute.Int("audio.size", len(audio)),
		)

		settings, err := ctrl.svc.Group.GetSettings(ctx)
		if err != nil {
			recordCtrlSpanError(span, err)
			log.Err(err).Msg("failed to load group settings")
			return validate.NewRequestError(err, http.StatusInternalServerError)
		}

		sttCfg, err := services.AssistantSTTConfig(settings)
		if err != nil {
			return validate.NewRequestError(err, http.StatusConflict)
		}

		// The STT API key must never end up in logs; the error values returned
		// by TranscribeAudio only carry status codes and response snippets.
		transcript, err := ctrl.svc.AI.TranscribeAudio(ctx, audio, mimeType, sttCfg)
		if err != nil {
			recordCtrlSpanError(span, err)
			log.Err(err).Msg("assistant speech-to-text failed")
			return validate.NewRequestError(err, http.StatusBadGateway)
		}

		reply, err := ctrl.svc.AI.ParseAssistantCommand(ctx, ctx.GID, history, transcript)
		if err != nil {
			recordCtrlSpanError(span, err)
			if errors.Is(err, services.ErrAIDisabled) {
				return validate.NewRequestError(err, http.StatusServiceUnavailable)
			}
			log.Err(err).Msg("assistant command parsing failed")
			return validate.NewRequestError(err, http.StatusBadGateway)
		}

		return server.JSON(w, http.StatusOK, assistantVoiceResponse{
			Transcript: transcript,
			Reply:      reply.Reply,
			Actions:    reply.Actions,
		})
	}
}
