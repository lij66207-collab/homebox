package services

import (
	"context"

	"github.com/google/uuid"
	"github.com/sysadminsmedia/homebox/backend/internal/data/repo"
)

const defaultLocationGarage = "车库"

// ensureDefaultEntityTypes guarantees a freshly created group has the two
// baseline entity types ("Item" and "Location"). The frontend create dialogs
// require selecting an existing type, so without these a brand-new group can't
// create items or locations at all. GetDefault creates each type if missing.
func ensureDefaultEntityTypes(ctx context.Context, repos *repo.AllRepos, gid uuid.UUID) error {
	for _, isLocation := range []bool{false, true} {
		if _, err := repos.EntityTypes.GetDefault(ctx, gid, isLocation); err != nil {
			return err
		}
	}
	return nil
}

func defaultLocations() []repo.EntityCreate {
	return []repo.EntityCreate{
		{
			Name: "客厅",
		},
		{
			Name: defaultLocationGarage,
		},
		{
			Name: "厨房",
		},
		{
			Name: "卧室",
		},
		{
			Name: "卫生间",
		},
		{
			Name: "书房",
		},
		{
			Name: "阁楼",
		},
		{
			Name: "地下室",
		},
	}
}

func defaultTags() []repo.TagCreate {
	return []repo.TagCreate{
		{
			Name: "家电",
		},
		{
			Name: "物联网",
		},
		{
			Name: "电子产品",
		},
		{
			Name: "服务器",
		},
		{
			Name: "通用",
		},
		{
			Name: "重要",
		},
	}
}
