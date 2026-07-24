package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent"
	"github.com/sysadminsmedia/homebox/backend/internal/data/repo"
)

func newAdminTestUser(t *testing.T) (repo.UserOut, repo.Group) {
	t.Helper()
	ctx := context.Background()

	group, err := tRepos.Groups.GroupCreate(ctx, "admin-test-"+fk.Str(6), uuid.Nil)
	require.NoError(t, err)

	password := fk.Str(20)
	usr, err := tRepos.Users.Create(ctx, repo.UserCreate{
		Name:           fk.Str(10),
		Email:          fk.Email(),
		Password:       &password,
		DefaultGroupID: group.ID,
	})
	require.NoError(t, err)

	return usr, group
}

func TestAdminDeleteUser_SelfRejected(t *testing.T) {
	ctx := context.Background()
	usr, _ := newAdminTestUser(t)

	err := tSvc.User.AdminDeleteUser(ctx, usr.ID, usr.ID)
	require.ErrorIs(t, err, ErrorAdminDeleteSelf)
}

func TestAdminDeleteUser_SoleMemberGroupCascade(t *testing.T) {
	ctx := context.Background()
	usr, group := newAdminTestUser(t)

	err := tSvc.User.AdminDeleteUser(ctx, uuid.New(), usr.ID)
	require.NoError(t, err)

	// user is gone
	_, err = tRepos.Users.GetOneID(ctx, usr.ID)
	require.Error(t, err)

	// sole-member group is gone too
	_, err = tClient.Group.Get(ctx, group.ID)
	require.Error(t, err)
	assert.True(t, ent.IsNotFound(err))
}

func TestAdminDeleteUser_SharedGroupKeepsGroup(t *testing.T) {
	ctx := context.Background()

	// owner of the shared group
	_, group := newAdminTestUser(t)

	// second member of the same group — the deletion target
	password := fk.Str(20)
	target, err := tRepos.Users.Create(ctx, repo.UserCreate{
		Name:           fk.Str(10),
		Email:          fk.Email(),
		Password:       &password,
		DefaultGroupID: group.ID,
	})
	require.NoError(t, err)

	require.NoError(t, tSvc.User.AdminDeleteUser(ctx, uuid.New(), target.ID))

	// target user is gone
	_, err = tRepos.Users.GetOneID(ctx, target.ID)
	require.Error(t, err)

	// shared group survives
	_, err = tClient.Group.Get(ctx, group.ID)
	require.NoError(t, err)

	// membership removed
	members, err := tRepos.Users.GetUsersByGroupID(ctx, group.ID)
	require.NoError(t, err)
	for _, m := range members {
		assert.NotEqual(t, target.ID, m.ID)
	}
}

func TestListUsers_ContainsFixtureUser(t *testing.T) {
	ctx := context.Background()
	usr, _ := newAdminTestUser(t)

	users, err := tSvc.User.ListUsers(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, users)

	found := false
	for _, u := range users {
		if u.ID == usr.ID {
			found = true
			break
		}
	}
	assert.True(t, found, "expected admin-created user in ListUsers result")
}
