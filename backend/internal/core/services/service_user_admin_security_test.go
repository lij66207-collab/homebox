package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdminSetPassword(t *testing.T) {
	ctx := context.Background()
	usr, _ := newAdminTestUser(t)

	// Too short → rejected
	require.ErrorIs(t, tSvc.User.AdminSetPassword(ctx, usr.ID, "123"), ErrorPasswordTooShort)

	// Set a new password → login with it succeeds
	require.NoError(t, tSvc.User.AdminSetPassword(ctx, usr.ID, "brand-new-password"))

	detail, err := tSvc.User.Login(ctx, usr.Email, "brand-new-password", false)
	require.NoError(t, err)
	assert.NotEmpty(t, detail.Raw)
}

func TestAdminSetDisabled(t *testing.T) {
	ctx := context.Background()
	usr, _ := newAdminTestUser(t)

	// Cannot disable yourself
	require.ErrorIs(t, tSvc.User.AdminSetDisabled(ctx, usr.ID, usr.ID, true), ErrorAdminDisableSelf)

	// Disable → login rejected
	require.NoError(t, tSvc.User.AdminSetDisabled(ctx, uuid.New(), usr.ID, true))
	_, err := tSvc.User.Login(ctx, usr.Email, "whatever", false)
	require.ErrorIs(t, err, ErrorInvalidLogin)

	// Re-enable → login works again (set a known password first)
	require.NoError(t, tSvc.User.AdminSetDisabled(ctx, uuid.New(), usr.ID, false))
	require.NoError(t, tSvc.User.AdminSetPassword(ctx, usr.ID, "re-enabled-password"))
	_, err = tSvc.User.Login(ctx, usr.Email, "re-enabled-password", false)
	require.NoError(t, err)
}
