package v1

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hay-kot/httpkit/errchain"
	"github.com/hay-kot/httpkit/server"
	"github.com/rs/zerolog/log"
	"github.com/sysadminsmedia/homebox/backend/internal/core/services"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/validate"
	"go.opentelemetry.io/otel/attribute"
)

// AdminUserCreateRequest is the payload for creating a user via the admin API.
type AdminUserCreateRequest struct {
	Name     string `json:"name"     validate:"required"`
	Email    string `json:"email"    validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

// AdminResetLinkResponse carries a one-time password reset link for an admin
// to hand to the user out-of-band.
type AdminResetLinkResponse struct {
	Link string `json:"link"`
}

// AdminSetPasswordRequest sets a user's password directly.
type AdminSetPasswordRequest struct {
	Password string `json:"password" validate:"required"`
}

// AdminSetDisabledRequest toggles a user's disabled flag.
type AdminSetDisabledRequest struct {
	Disabled bool `json:"disabled"`
}

// HandleAdminUsersList godoc
//
//	@Summary	List all users (superuser only)
//	@Tags		Admin
//	@Produce	json
//	@Success	200	{object}	[]repo.UserOut
//	@Failure	403	{string}	string	"Not a superuser"
//	@Router		/v1/admin/users [GET]
//	@Security	Bearer
func (ctrl *V1Controller) HandleAdminUsersList() errchain.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		spanCtx, span := startEntityCtrlSpan(r.Context(), "controller.V1.HandleAdminUsersList")
		defer span.End()

		users, err := ctrl.svc.User.ListUsers(spanCtx)
		if err != nil {
			recordCtrlSpanError(span, err)
			log.Err(err).Msg("failed to list users")
			return validate.NewRequestError(err, http.StatusInternalServerError)
		}

		return server.JSON(w, http.StatusOK, WrapResults(users))
	}
}

// HandleAdminUsersCreate godoc
//
//	@Summary	Create a user (superuser only)
//	@Tags		Admin
//	@Accept		json
//	@Produce	json
//	@Param		payload	body		AdminUserCreateRequest	true	"User Data"
//	@Success	201		{object}	Wrapped{item=repo.UserOut}
//	@Failure	403		{string}	string	"Not a superuser"
//	@Failure	409		{string}	string	"Email already in use"
//	@Router		/v1/admin/users [POST]
//	@Security	Bearer
func (ctrl *V1Controller) HandleAdminUsersCreate() errchain.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		spanCtx, span := startEntityCtrlSpan(r.Context(), "controller.V1.HandleAdminUsersCreate")
		defer span.End()

		body := AdminUserCreateRequest{}
		if err := server.Decode(r, &body); err != nil {
			recordCtrlSpanError(span, err)
			return validate.NewRequestError(err, http.StatusBadRequest)
		}

		body.Email = strings.ToLower(strings.TrimSpace(body.Email))
		body.Name = strings.TrimSpace(body.Name)

		if body.Name == "" || body.Email == "" {
			return validate.NewRequestError(errors.New("name and email are required"), http.StatusBadRequest)
		}
		if len(body.Password) < services.PasswordMinLength {
			return validate.NewRequestError(fmt.Errorf("password must be at least %d characters", services.PasswordMinLength), http.StatusBadRequest)
		}
		if ctrl.svc.User.ExistsByEmail(spanCtx, body.Email) {
			span.SetAttributes(attribute.String("create.outcome", "email_in_use"))
			return validate.NewRequestError(errors.New("email already in use"), http.StatusConflict)
		}

		usr, err := ctrl.svc.User.RegisterUser(spanCtx, services.UserRegistration{
			Name:     body.Name,
			Email:    body.Email,
			Password: body.Password,
		})
		if err != nil {
			recordCtrlSpanError(span, err)
			log.Err(err).Msg("admin failed to create user")
			return validate.NewRequestError(err, http.StatusInternalServerError)
		}
		span.SetAttributes(
			attribute.String("create.outcome", "success"),
			attribute.String("user.id", usr.ID.String()),
		)

		return server.JSON(w, http.StatusCreated, Wrap(usr))
	}
}

// HandleAdminUsersDelete godoc
//
//	@Summary	Delete a user (superuser only)
//	@Tags		Admin
//	@Param		id	path	string	true	"User ID"
//	@Success	204
//	@Failure	400	{string}	string	"Cannot delete own account via admin API"
//	@Failure	403	{string}	string	"Not a superuser"
//	@Router		/v1/admin/users/{id} [DELETE]
//	@Security	Bearer
func (ctrl *V1Controller) HandleAdminUsersDelete() errchain.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		spanCtx, span := startEntityCtrlSpan(r.Context(), "controller.V1.HandleAdminUsersDelete")
		defer span.End()

		if ctrl.isDemo {
			return validate.NewRequestError(errors.New("user deletion is disabled in demo mode"), http.StatusForbidden)
		}

		targetID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			return validate.NewRequestError(err, http.StatusBadRequest)
		}

		actor := services.UseUserCtx(spanCtx)
		if actor == nil || actor.ID == uuid.Nil {
			return validate.NewRequestError(errors.New("Unauthorized"), http.StatusUnauthorized)
		}

		err = ctrl.svc.User.AdminDeleteUser(spanCtx, actor.ID, targetID)
		if err != nil {
			if errors.Is(err, services.ErrorAdminDeleteSelf) {
				return validate.NewRequestError(err, http.StatusBadRequest)
			}
			recordCtrlSpanError(span, err)
			log.Err(err).Msg("admin failed to delete user")
			return validate.NewRequestError(err, http.StatusInternalServerError)
		}

		return server.JSON(w, http.StatusNoContent, nil)
	}
}

// HandleAdminUsersResetLink godoc
//
//	@Summary	Generate a one-time password reset link for a user (superuser only)
//	@Tags		Admin
//	@Produce	json
//	@Param		id	path		string	true	"User ID"
//	@Success	200	{object}	Wrapped{item=AdminResetLinkResponse}
//	@Failure	403	{string}	string	"Not a superuser"
//	@Failure	404	{string}	string	"User not found"
//	@Router		/v1/admin/users/{id}/reset-link [POST]
//	@Security	Bearer
func (ctrl *V1Controller) HandleAdminUsersResetLink() errchain.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		spanCtx, span := startEntityCtrlSpan(r.Context(), "controller.V1.HandleAdminUsersResetLink")
		defer span.End()

		targetID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			return validate.NewRequestError(err, http.StatusBadRequest)
		}

		target, err := ctrl.repo.Users.GetOneID(spanCtx, targetID)
		if err != nil {
			return validate.NewRequestError(errors.New("user not found"), http.StatusNotFound)
		}

		baseURL := strings.TrimSuffix(ctrl.config.Options.Hostname, "/")
		if baseURL == "" {
			baseURL = strings.TrimSuffix(r.Header.Get("Origin"), "/")
		}

		link, err := ctrl.svc.User.GenerateResetLink(spanCtx, target.Email, baseURL)
		if err != nil {
			recordCtrlSpanError(span, err)
			log.Err(err).Msg("admin failed to generate reset link")
			return validate.NewRequestError(err, http.StatusInternalServerError)
		}

		return server.JSON(w, http.StatusOK, Wrap(AdminResetLinkResponse{Link: link}))
	}
}

// HandleAdminUsersSetPassword godoc
//
//	@Summary	Set a user's password directly (superuser only)
//	@Tags		Admin
//	@Accept		json
//	@Param		id		path	string						true	"User ID"
//	@Param		payload	body	AdminSetPasswordRequest	true	"New password"
//	@Success	204
//	@Failure	400	{string}	string	"Password too short"
//	@Failure	403	{string}	string	"Not a superuser"
//	@Router		/v1/admin/users/{id}/password [PUT]
//	@Security	Bearer
func (ctrl *V1Controller) HandleAdminUsersSetPassword() errchain.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		spanCtx, span := startEntityCtrlSpan(r.Context(), "controller.V1.HandleAdminUsersSetPassword")
		defer span.End()

		targetID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			return validate.NewRequestError(err, http.StatusBadRequest)
		}

		body := AdminSetPasswordRequest{}
		if err := server.Decode(r, &body); err != nil {
			return validate.NewRequestError(err, http.StatusBadRequest)
		}

		err = ctrl.svc.User.AdminSetPassword(spanCtx, targetID, body.Password)
		if err != nil {
			if errors.Is(err, services.ErrorPasswordTooShort) {
				return validate.NewRequestError(err, http.StatusBadRequest)
			}
			recordCtrlSpanError(span, err)
			log.Err(err).Msg("admin failed to set password")
			return validate.NewRequestError(err, http.StatusInternalServerError)
		}

		return server.JSON(w, http.StatusNoContent, nil)
	}
}

// HandleAdminUsersSetDisabled godoc
//
//	@Summary	Disable or re-enable a user account (superuser only)
//	@Tags		Admin
//	@Accept		json
//	@Param		id		path	string					true	"User ID"
//	@Param		payload	body	AdminSetDisabledRequest	true	"Disabled flag"
//	@Success	204
//	@Failure	400	{string}	string	"Cannot disable own account"
//	@Failure	403	{string}	string	"Not a superuser"
//	@Router		/v1/admin/users/{id}/disabled [PUT]
//	@Security	Bearer
func (ctrl *V1Controller) HandleAdminUsersSetDisabled() errchain.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		spanCtx, span := startEntityCtrlSpan(r.Context(), "controller.V1.HandleAdminUsersSetDisabled")
		defer span.End()

		targetID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			return validate.NewRequestError(err, http.StatusBadRequest)
		}

		body := AdminSetDisabledRequest{}
		if err := server.Decode(r, &body); err != nil {
			return validate.NewRequestError(err, http.StatusBadRequest)
		}

		actor := services.UseUserCtx(spanCtx)
		if actor == nil || actor.ID == uuid.Nil {
			return validate.NewRequestError(errors.New("Unauthorized"), http.StatusUnauthorized)
		}

		err = ctrl.svc.User.AdminSetDisabled(spanCtx, actor.ID, targetID, body.Disabled)
		if err != nil {
			if errors.Is(err, services.ErrorAdminDisableSelf) {
				return validate.NewRequestError(err, http.StatusBadRequest)
			}
			recordCtrlSpanError(span, err)
			log.Err(err).Msg("admin failed to set disabled flag")
			return validate.NewRequestError(err, http.StatusInternalServerError)
		}

		return server.JSON(w, http.StatusNoContent, nil)
	}
}
