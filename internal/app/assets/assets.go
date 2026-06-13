package assets

import (
	"errors"
	"fmt"

	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/assets/animation"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/assets/badge"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/assets/decal"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/assets/developerproduct"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/assets/gamepass"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/assets/mesh"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/assets/shared/clientutils"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/assets/shared/permissions"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/assets/sound"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/checkpoint"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/context"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/request"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/response"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/color"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/console"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox/clientpool"
)

var assetModules = map[string]func(ctx *context.Context, r *request.Request){
	"Animation":        animation.Reupload,
	"Decal":            decal.Reupload,
	"Mesh":             mesh.Reupload,
	"Sound":            sound.Reupload,
	"GamePass":         gamepass.Reupload,
	"DeveloperProduct": developerproduct.Reupload,
	"Badge":            badge.Reupload,
}

func NewReuploadHandlerWithType(assetType string, pool *clientpool.Pool, r *request.RawRequest, resp *response.Response) (func() error, error) {
	reupload, exists := assetModules[assetType]
	if !exists {
		return func() error { return nil }, errors.New(assetType + " module does not exist")
	}

	return func() error {
		ctx := context.New(pool, resp)

		ctx.Logger.Verbose(fmt.Sprintf("Reupload request: type=%s, %d id(s), creatorId=%d, group=%t, placeId=%d, exportJSON=%t",
			r.AssetType, len(r.IDs), r.CreatorID, r.IsGroup, r.PlaceID, r.ExportJSON))

		console.ClearScreen()

		fmt.Println("Getting current place details...")
		req, err := request.FromRawRequest(ctx.Client, r)
		console.ClearScreen()
		if err != nil {
			return err
		}
		ctx.Logger.Verbose(fmt.Sprintf("Resolved place %d to universe %d", req.PlaceID, req.UniverseID))

		store, err := checkpoint.Load(checkpoint.Key(r.CreatorID, r.AssetType, r.IsGroup))
		if err != nil {
			fmt.Println("Note:", err)
		}
		ctx.Checkpoint = store
		ctx.Logger.Verbose(fmt.Sprintf("Checkpoint loaded: %d asset(s) already completed in a previous run", store.Count()))

		ctx = ensureEditAccess(ctx, req, resp)

		if done := store.Count(); done > 0 {
			fmt.Printf("Resuming: %d asset(s) already reuploaded will be skipped.\n", done)
		}

		reupload(ctx, req)

		p := resp.Progress()
		if !ctx.Cancelled() && p.StopReason == "" && p.Failed == 0 {
			if err := store.Cleanup(); err != nil {
				fmt.Println("Note: failed to remove checkpoint:", err)
			}
		}
		return nil
	}, nil
}

func ensureEditAccess(ctx *context.Context, req *request.Request, resp *response.Response) *context.Context {
	clients := ctx.Clients.All()
	kept := make([]*roblox.Client, 0, len(clients))

	console.ClearScreen()
	fmt.Println("Checking that every account can edit the experience...")
	fmt.Println("(All accounts must have edit/create-item permission on this game.)")
	fmt.Println()

	var lastErr error
	for _, client := range clients {
		name := client.UserInfo.Username
		if name == "" {
			name = "an account"
		}

		err := permissions.CanEditUniverse(client, req)
		if err == nil {
			kept = append(kept, client)
			ctx.Logger.Verbose(fmt.Sprintf("%s can edit the experience", name))
			continue
		}

		lastErr = err
		color.Warn.Println(fmt.Sprintf("Skipping %s - cannot edit this game: %v", name, err))
	}

	if len(kept) == 0 {
		primary := ctx.Clients.Primary()
		msg := "no account can edit this experience"
		if lastErr != nil {
			msg = lastErr.Error()
		}
		clientutils.GetNewCookie(ctx, req, primary, msg)
		kept = append(kept, primary)
	}

	if len(kept) == ctx.Clients.Len() {
		ctx.Logger.Verbose(fmt.Sprintf("All %d account(s) can edit the experience", len(kept)))
		return ctx
	}

	color.Warn.Println(fmt.Sprintf("Continuing with %d of %d account(s).", len(kept), len(clients)))
	newCtx := context.New(clientpool.New(kept), resp)
	newCtx.Checkpoint = ctx.Checkpoint
	return newCtx
}

func DoesModuleExist(m string) bool {
	_, exists := assetModules[m]
	return exists
}
