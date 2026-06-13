package assetutils

import (
	"errors"

	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/context"
)

var ErrStopped = errors.New("reupload stopped")

func StopForModeration(ctx *context.Context) {
	if ctx.Cancel() {
		ctx.Response.SetModerated()
		ctx.Response.SetStopped("your account has been moderated by Roblox")
		ctx.Logger.Error("Your account has been moderated by Roblox - stopping reupload.")
	}
}
