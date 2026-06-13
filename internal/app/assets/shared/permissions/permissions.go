package permissions

import (
	"errors"

	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/request"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox/develop"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox/groups"
)

var (
	ErrNotMember              = errors.New("your account is not a member of this group")
	ErrNoCreateItemPermission = errors.New("your account does not have permission to create items for this group")
	ErrNoManageGroupGames     = errors.New("your account does not have permission to manage this group's experiences")
	ErrNoEditPermission       = errors.New("your account does not have permission to edit this experience")
)

func canEditGroup(c *roblox.Client, groupID int64) error {
	groupMembership, err := groups.Membership(c, groupID)
	if err != nil {
		return err
	}

	if groupMembership.UserRole.Role.Name == "Guest" {
		return ErrNotMember
	}

	groupPermissions := groupMembership.Permissions.GroupEconomyPermissions
	if canCreateItems := groupPermissions.CreateItems; !canCreateItems {
		return ErrNoCreateItemPermission
	}

	if canManageGames := groupPermissions.ManageGroupGames; !canManageGames {
		return ErrNoManageGroupGames
	}

	return nil
}

func CanEditUniverse(client *roblox.Client, r *request.Request) error {
	if r.IsGroup {
		return canEditGroup(client, r.CreatorID)
	}

	_, err := develop.TeamCreateSettings(client, r.UniverseID)
	if err == develop.TeamCreateSettingsErrors.ErrAuthorizationDenied {
		return ErrNoEditPermission
	}

	return err
}
