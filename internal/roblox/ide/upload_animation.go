package ide

import (
	"bytes"
	"errors"

	"github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox"
)

var UploadAnimationErrors = struct {
	ErrNotLoggedIn       error
	ErrTokenInvalid      error
	ErrInappropriateName error
}{
	ErrNotLoggedIn:       errors.New("you appear to be signed out - your Roblox cookie may have expired"),
	ErrTokenInvalid:      errors.New("Roblox rejected the request's security token (it will refresh and retry automatically)"),
	ErrInappropriateName: errors.New("Roblox moderation flagged the animation's name or description"),
}

func NewUploadAnimationHandler(
	c *roblox.Client,
	name,
	description string,
	data *bytes.Buffer,
	groupID ...int64,
) (func() (int64, error), error) {
	var group int64
	if len(groupID) > 0 {
		group = groupID[0]
	}
	currentName := name

	return func() (int64, error) {
		req, err := newCreateAssetRequest(
			"Animation",
			currentName,
			description,
			data,
			"model/x-rbxm",
			func() int64 {
				if group > 0 {
					return group
				}
				return c.UserInfo.ID
			}(),
			group > 0,
		)
		if err != nil {
			return 0, err
		}

		id, err := executeCreateAsset(c, req, UploadAnimationErrors.ErrTokenInvalid, UploadAnimationErrors.ErrNotLoggedIn)
		if err == nil {
			return id, nil
		}

		if errors.Is(err, ErrAccountModerated) {
			return 0, err
		}

		if isInappropriateError(err.Error()) {
			currentName = "[Censored]"
			return 0, UploadAnimationErrors.ErrInappropriateName
		}

		return 0, err
	}, nil
}
