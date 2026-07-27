package v1

import (
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hay-kot/httpkit/errchain"
	"github.com/hay-kot/httpkit/server"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/sysadminsmedia/homebox/backend/internal/core/services"
	"github.com/sysadminsmedia/homebox/backend/internal/data/repo"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/validate"
	"github.com/sysadminsmedia/homebox/backend/internal/web/adapters"
	"go.opentelemetry.io/otel/attribute"
)

type (
	GroupInvitationCreate struct {
		Uses      int       `json:"uses"      validate:"required,min=1,max=100"`
		ExpiresAt time.Time `json:"expiresAt"`
	}

	GroupInvitation struct {
		ID        uuid.UUID `json:"id"`
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expiresAt"`
		Uses      int       `json:"uses"`
	}

	GroupAcceptInvitationResponse struct {
		ID   uuid.UUID `json:"id"`
		Name string    `json:"name"`
	}

	CreateRequest struct {
		Name string `json:"name" validate:"required"`
	}
)

// HandleGroupGet godoc
//
//	@Summary	Get Group
//	@Tags		Group
//	@Produce	json
//	@Success	200	{object}	repo.Group
//	@Router		/v1/groups [Get]
//	@Security	Bearer
func (ctrl *V1Controller) HandleGroupGet() errchain.HandlerFunc {
	fn := func(r *http.Request) (repo.Group, error) {
		auth := services.NewContext(r.Context())
		return ctrl.repo.Groups.GroupByID(auth, auth.GID)
	}

	return adapters.Command(fn, http.StatusOK)
}

// HandleGroupUpdate godoc
//
//	@Summary	Update Group
//	@Tags		Group
//	@Produce	json
//	@Param		payload	body		repo.GroupUpdate	true	"User Data"
//	@Success	200		{object}	repo.Group
//	@Router		/v1/groups [Put]
//	@Security	Bearer
func (ctrl *V1Controller) HandleGroupUpdate() errchain.HandlerFunc {
	fn := func(r *http.Request, body repo.GroupUpdate) (repo.Group, error) {
		auth := services.NewContext(r.Context())

		ok := ctrl.svc.Currencies.IsSupported(body.Currency)
		if !ok {
			return repo.Group{}, validate.NewFieldErrors(
				validate.NewFieldError("currency", "currency '"+body.Currency+"' is not supported"),
			)
		}

		return ctrl.svc.Group.UpdateGroup(auth, body)
	}

	return adapters.Action(fn, http.StatusOK)
}

// HandleGroupInvitationsCreate godoc
//
//	@Summary	Create Group Invitation
//	@ID			groupInvitationCreate
//	@Tags		Group
//	@Produce	json
//	@Param		payload	body		GroupInvitationCreate	true	"User Data"
//	@Success	200		{object}	GroupInvitation
//	@Router		/v1/groups/invitations [Post]
//	@Security	Bearer
func (ctrl *V1Controller) HandleGroupInvitationsCreate() errchain.HandlerFunc {
	fn := func(r *http.Request, body GroupInvitationCreate) (GroupInvitation, error) {
		if body.ExpiresAt.IsZero() {
			body.ExpiresAt = time.Now().Add(time.Hour * 24)
		}

		auth := services.NewContext(r.Context())

		invitation, token, err := ctrl.svc.Group.NewInvitation(auth, body.Uses, body.ExpiresAt)
		if err != nil {
			return GroupInvitation{}, err
		}

		return GroupInvitation{
			ID:        invitation.ID,
			Token:     token,
			ExpiresAt: invitation.ExpiresAt,
			Uses:      invitation.Uses,
		}, nil
	}

	return adapters.Action(fn, http.StatusCreated)
}

// HandleGroupsGetAll godoc
//
//	@Summary	Get All Groups
//	@Tags		Group
//	@Produce	json
//	@Success	200	{object}	[]repo.Group
//	@Router		/v1/groups/all [Get]
//	@Security	Bearer
func (ctrl *V1Controller) HandleGroupsGetAll() errchain.HandlerFunc {
	fn := func(r *http.Request) ([]repo.Group, error) {
		auth := services.NewContext(r.Context())
		return ctrl.repo.Groups.GetAllGroups(auth, auth.UID)
	}

	return adapters.Command(fn, http.StatusOK)
}

// HandleGroupCreate godoc
//
//	@Summary	Create Group
//	@Tags		Group
//	@Produce	json
//	@Param		payload	body		CreateRequest	true	"Create group request"
//	@Success	201		{object}	repo.Group
//	@Router		/v1/groups [Post]
//	@Security	Bearer
func (ctrl *V1Controller) HandleGroupCreate() errchain.HandlerFunc {
	fn := func(r *http.Request, body CreateRequest) (repo.Group, error) {
		auth := services.NewContext(r.Context())
		return ctrl.svc.Group.CreateGroup(auth, body.Name)
	}

	return adapters.Action(fn, http.StatusCreated)
}

// HandleGroupDelete godoc
//
//	@Summary	Delete Group
//	@Tags		Group
//	@Produce	json
//	@Success	204
//	@Router		/v1/groups [Delete]
//	@Security	Bearer
func (ctrl *V1Controller) HandleGroupDelete() errchain.HandlerFunc {
	fn := func(r *http.Request) (any, error) {
		auth := services.NewContext(r.Context())

		// Get the current user to check their groups
		currentUser, err := ctrl.repo.Users.GetOneID(auth, auth.UID)
		if err != nil {
			return nil, err
		}

		// Safeguard: prevent deleting if this is the user's only group
		if len(currentUser.GroupIDs) <= 1 {
			return nil, validate.NewRequestError(errors.New("cannot delete the only group you are a member of"), http.StatusBadRequest)
		}

		// If the group being deleted is the user's default group, reassign to another group
		if currentUser.DefaultGroupID == auth.GID {
			// Find another group the user is a member of
			newDefaultGroupID, _ := lo.Find(currentUser.GroupIDs, func(gid uuid.UUID) bool {
				return gid != auth.GID
			})

			// Update the user's default group
			if err := ctrl.repo.Users.UpdateDefaultGroup(auth, auth.UID, newDefaultGroupID); err != nil {
				return nil, err
			}
		}

		err = ctrl.svc.Group.DeleteGroup(auth)
		return nil, err
	}

	return adapters.Command(fn, http.StatusNoContent)
}

// HandleGroupInvitationsGetAll godoc
//
//	@Summary	Get All Group Invitations
//	@Tags		Group
//	@Produce	json
//	@Success	200	{object}	[]repo.GroupInvitation
//	@Router		/v1/groups/invitations [Get]
//	@Security	Bearer
func (ctrl *V1Controller) HandleGroupInvitationsGetAll() errchain.HandlerFunc {
	fn := func(r *http.Request) ([]repo.GroupInvitation, error) {
		auth := services.NewContext(r.Context())
		return ctrl.repo.Groups.InvitationGetAll(auth, auth.GID)
	}

	return adapters.Command(fn, http.StatusOK)
}

// HandleGroupMembersGetAll godoc
//
//	@Summary	Get All Group Members
//	@Tags		Group
//	@Produce	json
//	@Success	200	{object}	[]repo.UserSummary
//	@Router		/v1/groups/members [Get]
//	@Security	Bearer
func (ctrl *V1Controller) HandleGroupMembersGetAll() errchain.HandlerFunc {
	fn := func(r *http.Request) ([]repo.UserSummary, error) {
		auth := services.NewContext(r.Context())
		return ctrl.repo.Users.GetUsersByGroupID(auth, auth.GID)
	}

	return adapters.Command(fn, http.StatusOK)
}

// HandleGroupMemberRemove godoc
//
//	@Summary	Remove User from Group
//	@Tags		Group
//	@Produce	json
//	@Param		user_id	path	string	true	"User ID"
//	@Success	204
//	@Router		/v1/groups/members/{user_id} [Delete]
//	@Security	Bearer
func (ctrl *V1Controller) HandleGroupMemberRemove() errchain.HandlerFunc {
	fn := func(r *http.Request, userID uuid.UUID) (any, error) {
		auth := services.NewContext(r.Context())

		// Safeguard: prevent user from removing themselves
		if userID == auth.UID {
			return nil, validate.NewRequestError(errors.New("cannot remove yourself from the group"), http.StatusBadRequest)
		}

		// Safeguard: prevent removing the last member
		members, err := ctrl.repo.Users.GetUsersByGroupID(auth, auth.GID)
		if err != nil {
			return nil, err
		}
		if len(members) <= 1 {
			return nil, validate.NewRequestError(errors.New("cannot remove the last member from the group"), http.StatusBadRequest)
		}

		err = ctrl.svc.Group.RemoveMember(auth, userID)
		return nil, err
	}

	return adapters.CommandID("user_id", fn, http.StatusNoContent)
}

// HandleGroupInvitationsDelete godoc
//
//	@Summary	Delete Group Invitation
//	@Tags		Group
//	@Produce	json
//	@Param		id	path	string	true	"Invitation ID"
//	@Success	204
//	@Router		/v1/groups/invitations/{id} [Delete]
//	@Security	Bearer
func (ctrl *V1Controller) HandleGroupInvitationsDelete() errchain.HandlerFunc {
	fn := func(r *http.Request, id uuid.UUID) (any, error) {
		auth := services.NewContext(r.Context())
		err := ctrl.svc.Group.DeleteInvitation(auth, id)
		return nil, err
	}

	return adapters.CommandID("id", fn, http.StatusNoContent)
}

// HandleGroupInvitationsAccept godoc
//
//	@Summary	Accept Group Invitation
//	@ID			groupInvitationAccept
//	@Tags		Group
//	@Produce	json
//	@Param		id	path		string	true	"Invitation Token"
//	@Success	200	{object}	GroupAcceptInvitationResponse
//	@Router		/v1/groups/invitations/{id} [Post]
//	@Security	Bearer
func (ctrl *V1Controller) HandleGroupInvitationsAccept() errchain.HandlerFunc {
	fn := func(r *http.Request) (GroupAcceptInvitationResponse, error) {
		token := chi.URLParam(r, "id")
		if token == "" {
			return GroupAcceptInvitationResponse{}, validate.NewRequestError(errors.New("token is required"), http.StatusBadRequest)
		}

		auth := services.NewContext(r.Context())
		group, err := ctrl.svc.Group.AcceptInvitation(auth, token)
		if err != nil {
			if errors.Is(err, errors.New("user already a member of this group")) {
				return GroupAcceptInvitationResponse{}, validate.NewRequestError(err, http.StatusBadRequest)
			}
			return GroupAcceptInvitationResponse{}, err
		}

		return GroupAcceptInvitationResponse{ID: group.ID, Name: group.Name}, nil
	}

	return adapters.Command(fn, http.StatusOK)
}

// HandleGroupSettingsGet godoc
//
//	@Summary	Get group settings
//	@Tags		Group
//	@Produce	json
//	@Success	200	{object}	Wrapped{item=map[string]interface{}}
//	@Router		/v1/group/settings [GET]
//	@Security	Bearer
func (ctrl *V1Controller) HandleGroupSettingsGet() errchain.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		spanCtx, span := startEntityCtrlSpan(r.Context(), "controller.V1.HandleGroupSettingsGet")
		defer span.End()

		auth := services.NewContext(spanCtx)
		settings, err := ctrl.svc.Group.GetSettings(auth)
		if err != nil {
			recordCtrlSpanError(span, err)
			return validate.NewRequestError(err, http.StatusInternalServerError)
		}
		span.SetAttributes(attribute.Int("settings.keys.count", len(settings)))

		w.Header().Set("Cache-Control", "no-store")
		return server.JSON(w, http.StatusOK, Wrap(services.RedactGroupSettings(settings)))
	}
}

// HandleGroupSettingsUpdate godoc
//
//	@Summary	Update group settings
//	@Tags		Group
//	@Produce	json
//	@Success	200	{object}	Wrapped{item=map[string]interface{}}
//	@Router		/v1/group/settings [PUT]
//	@Param		payload	body	map[string]interface{}	true	"Settings Data"
//	@Security	Bearer
func (ctrl *V1Controller) HandleGroupSettingsUpdate() errchain.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		spanCtx, span := startEntityCtrlSpan(r.Context(), "controller.V1.HandleGroupSettingsUpdate")
		defer span.End()

		r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
		var settings map[string]interface{}
		if err := server.Decode(r, &settings); err != nil {
			recordCtrlSpanError(span, err)
			span.SetAttributes(attribute.String("settings.outcome", "decode_failed"))
			log.Err(err).Msg("failed to decode group settings data")
			return validate.NewRequestError(err, http.StatusBadRequest)
		}
		span.SetAttributes(attribute.Int("settings.keys.count", len(settings)))

		auth := services.NewContext(spanCtx)
		updated, err := ctrl.svc.Group.SetSettings(auth, settings)
		if err != nil {
			recordCtrlSpanError(span, err)
			span.SetAttributes(attribute.String("settings.outcome", "set_failed"))
			return validate.NewRequestError(err, http.StatusInternalServerError)
		}
		span.SetAttributes(attribute.String("settings.outcome", "success"))

		w.Header().Set("Cache-Control", "no-store")
		return server.JSON(w, http.StatusOK, Wrap(services.RedactGroupSettings(updated)))
	}
}

// GroupTestServerChanRequest is the payload for the Server酱 test endpoint.
// SendKey is optional; when empty (or the redaction sentinel echoed back by
// the UI) the group's stored sendkey is used.
type GroupTestServerChanRequest struct {
	SendKey string `json:"sendkey"`
}

// HandleGroupTestServerChan godoc
//
//	@Summary	Send a Server酱 test push
//	@Tags		Group
//	@Produce	json
//	@Param		payload	body		GroupTestServerChanRequest	false	"Test payload; empty sendkey uses the stored one"
//	@Success	200		{object}	Wrapped{item=string}
//	@Router		/v1/group/settings/test-serverchan [Post]
//	@Security	Bearer
func (ctrl *V1Controller) HandleGroupTestServerChan() errchain.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		r.Body = http.MaxBytesReader(w, r.Body, 4*1024)

		// The body is optional; an empty one means "use the stored sendkey".
		var body GroupTestServerChanRequest
		if err := server.Decode(r, &body); err != nil && !errors.Is(err, io.EOF) {
			return validate.NewRequestError(err, http.StatusBadRequest)
		}

		auth := services.NewContext(r.Context())
		sendKey := body.SendKey
		if sendKey == "" || sendKey == services.RedactedSettingsValue {
			stored, err := ctrl.svc.Group.GetServerChanSendKey(auth)
			if err != nil {
				return validate.NewRequestError(err, http.StatusInternalServerError)
			}
			sendKey = stored
		}
		if sendKey == "" {
			return validate.NewRequestError(errors.New("no serverchan sendkey provided or stored"), http.StatusBadRequest)
		}

		err := services.SendServerChan(r.Context(), sendKey, "Homebox 测试消息", "这是一条来自 Homebox 的 Server酱 测试推送，说明 SendKey 配置正确。")
		if err != nil {
			// Log the failure without the sendkey; the API error message is
			// safe to return to the caller.
			log.Err(err).Msg("serverchan test push failed")
			return validate.NewRequestError(err, http.StatusBadGateway)
		}

		return server.JSON(w, http.StatusOK, Wrap("ok"))
	}
}
