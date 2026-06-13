package assetutils

import (
	"fmt"

	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/context"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox"
)

func NoteUploadFailure(ctx *context.Context, client *roblox.Client) {
	if evicted, remaining := ctx.Clients.ReportFailure(client); evicted {
		ctx.Logger.Warn(fmt.Sprintf("Dropping account %s after repeated upload failures - continuing with %d account(s).", client.UserInfo.Username, remaining))
	}
}
