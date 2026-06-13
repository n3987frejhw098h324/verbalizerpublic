package clientutils

import (
	"errors"
	"fmt"

	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/assets/shared/permissions"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/config"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/context"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/request"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/color"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/console"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox"
)

func GetNewCookie(ctx *context.Context, r *request.Request, client *roblox.Client, m string) {
	pauseController := ctx.PauseController

	if !pauseController.Pause() {
		pauseController.WaitIfPaused()
		return
	}

	name := client.UserInfo.Username
	if name == "" {
		name = "an account"
	}
	ctx.Logger.Verbose(fmt.Sprintf("Pausing run to get a new cookie for %s: %s", name, m))

	console.ClearScreen()

	inputErr := errors.New(m)
	for {
		fmt.Print(ctx.Logger.History.String())
		if username := client.UserInfo.Username; username != "" {
			color.Warn.Println("Account needing a new cookie: " + username)
		}
		color.Error.Println(inputErr)

		i, err := console.LongInput("ROBLOSECURITY: ")
		console.ClearScreen()
		if err != nil {
			inputErr = err
			continue
		}

		fmt.Println("Authenticating cookie...")
		err = client.SetCookie(i)
		console.ClearScreen()
		if err != nil {
			inputErr = err
			continue
		}

		fmt.Println("Checking if account can edit universe...")
		err = permissions.CanEditUniverse(client, r)
		console.ClearScreen()
		if err != nil {
			inputErr = err
			continue
		}

		break
	}

	fmt.Print(ctx.Logger.History.String())
	ctx.Logger.Verbose(fmt.Sprintf("Cookie refreshed for %s; resuming run", name))

	if err := saveAccounts(ctx); err != nil {
		ctx.Logger.Error("Failed to save cookie: ", err)
	}

	pauseController.Unpause()
}

func saveAccounts(ctx *context.Context) error {
	order := make([]int64, 0)
	byID := make(map[int64]config.Account)

	for _, a := range config.LoadAccounts() {
		if _, ok := byID[a.UserID]; !ok {
			order = append(order, a.UserID)
		}
		byID[a.UserID] = a
	}

	for _, c := range ctx.Clients.All() {
		id := c.UserInfo.ID
		if _, ok := byID[id]; !ok {
			order = append(order, id)
		}
		byID[id] = config.Account{Cookie: c.Cookie, APIKey: c.APIKey, UserID: id}
	}

	accounts := make([]config.Account, 0, len(order))
	for _, id := range order {
		accounts = append(accounts, byID[id])
	}
	return config.SaveAccounts(accounts)
}
