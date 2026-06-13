package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/config"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/color"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/console"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox/clientpool"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox/cloudauth"
)

var errInputClosed = errors.New("input stream closed before required input was provided")

var port = config.Get("port")

func main() {
	console.SetTitle("Verbalizer")
	console.ClearScreen()
	printBanner()
	verboseConfigDump()

	accounts := loadAccounts()

	accounts, err := manageAccounts(accounts)
	if err != nil {
		color.Error.Println(err)
		os.Exit(1)
	}

	if err := persistAccounts(accounts); err != nil {
		color.Error.Println("Failed to save accounts: ", err)
	} else {
		verbosef("Saved %d account(s) to config.ini", len(accounts))
	}

	clients := validClients(accounts)
	pool := clientpool.New(clients)
	for _, c := range clients {
		verbosef("Upload account ready: %s (id %d)", c.UserInfo.Username, c.UserInfo.ID)
	}

	console.ClearScreen()
	printBanner()
	if len(clients) == 1 {
		color.Success.Println("Authenticated 1 account. Ready for use.")
	} else {
		color.Success.Println(fmt.Sprintf("Authenticated %d accounts. Uploads will be spread across them.", len(clients)))
	}
	fmt.Println()
	fmt.Println("  Listening on http://localhost:" + port)
	fmt.Println("  Start a reupload using the plugin.")
	fmt.Println()

	if err := serve(pool); err != nil {
		log.Fatal(err)
	}
}

type account struct {
	cookie string
	apiKey string
	userID int64
	client *roblox.Client
}

func (a account) valid() bool { return a.client != nil }

func (a account) toConfig() config.Account {
	if a.client != nil {
		return config.Account{Cookie: a.client.Cookie, APIKey: a.client.APIKey, UserID: a.client.UserInfo.ID}
	}
	return config.Account{Cookie: a.cookie, APIKey: a.apiKey, UserID: a.userID}
}

func validClients(accounts []account) []*roblox.Client {
	clients := make([]*roblox.Client, 0, len(accounts))
	for _, a := range accounts {
		if a.valid() {
			clients = append(clients, a.client)
		}
	}
	return clients
}

func countValid(accounts []account) int {
	n := 0
	for _, a := range accounts {
		if a.valid() {
			n++
		}
	}
	return n
}

func loadAccounts() []account {
	stored := config.LoadAccounts()
	verbosef("Loading %d saved account(s)...", len(stored))
	accounts := make([]account, 0, len(stored))
	for _, acc := range stored {
		a := account{cookie: acc.Cookie, apiKey: acc.APIKey, userID: acc.UserID}

		fmt.Println("Authenticating saved account...")
		c, err := roblox.NewClient(acc.Cookie)
		if c != nil && err == nil {
			c.APIKey = strings.TrimSpace(acc.APIKey)
			if keyErr := ensureAPIKey(c); keyErr != nil {
				color.Error.Println(keyErr)
			}
			a.client = c
			a.userID = c.UserInfo.ID
			verbosef("Authenticated %s (id %d)", c.UserInfo.Username, c.UserInfo.ID)
		} else {
			verbosef("Saved account could not be authenticated (kept as expired): %v", err)
		}
		accounts = append(accounts, a)
	}
	return accounts
}

func manageAccounts(accounts []account) ([]account, error) {
	for {
		console.ClearScreen()
		printBanner()
		printAccounts(accounts)
		color.Info.Println("All accounts must have edit/create-item permission on the game you reupload assets on.")
		fmt.Println()

		if countValid(accounts) == 0 {
			fmt.Println("Add a working account to begin.")
			a, err := addAccount(accounts)
			if err != nil {
				return nil, err
			}
			if a.valid() {
				accounts = append(accounts, a)
			}
			continue
		}

		hasExpired := countValid(accounts) < len(accounts)
		prompt := "Press Enter to start, or type 'a' to add / 'r' to remove"
		if hasExpired {
			prompt += " / 'f' to refresh (drop expired)"
		}
		prompt += ": "

		choice, err := console.Input(prompt)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil, errInputClosed
			}
			continue
		}

		switch strings.ToLower(strings.TrimSpace(choice)) {
		case "", "c", "continue", "start", "s":
			return accounts, nil
		case "a", "add":
			a, err := addAccount(accounts)
			if err != nil {
				return nil, err
			}
			if a.valid() {
				accounts = append(accounts, a)
			}
		case "r", "remove":
			accounts = removeAccount(accounts)
		case "f", "refresh":
			accounts = refreshAccounts(accounts)
		}
	}
}

func printAccounts(accounts []account) {
	if len(accounts) == 0 {
		color.Warn.Println("No accounts added yet.")
		fmt.Println()
		return
	}
	fmt.Println(fmt.Sprintf("Accounts (%d):", len(accounts)))
	for i, a := range accounts {
		if a.valid() {
			fmt.Println(fmt.Sprintf("  %d. %s", i+1, a.client.UserInfo.Username))
		} else {
			color.Warn.Println(fmt.Sprintf("  %d. (expired - cookie no longer works)", i+1))
		}
	}
	fmt.Println()
}

func addAccount(existing []account) (account, error) {
	c, _ := roblox.NewClient("")
	if c == nil {
		return account{}, errors.New("failed to initialize Roblox client")
	}

	if err := getCookie(c); err != nil {
		return account{}, err
	}

	for _, e := range existing {
		if e.userID != 0 && e.userID == c.UserInfo.ID {
			console.ClearScreen()
			printBanner()
			color.Warn.Println("That account (" + c.UserInfo.Username + ") is already added.")
			fmt.Println()
			return account{}, nil
		}
	}

	if err := ensureAPIKey(c); err != nil {
		return account{}, err
	}
	return account{cookie: c.Cookie, apiKey: c.APIKey, userID: c.UserInfo.ID, client: c}, nil
}

func removeAccount(accounts []account) []account {
	if len(accounts) == 0 {
		return accounts
	}
	choice, err := console.Input("Remove which account? Enter its number (or press Enter to cancel): ")
	if err != nil {
		return accounts
	}
	choice = strings.TrimSpace(choice)
	if choice == "" {
		return accounts
	}
	n, err := strconv.Atoi(choice)
	if err != nil || n < 1 || n > len(accounts) {
		return accounts
	}
	return append(accounts[:n-1], accounts[n:]...)
}

func refreshAccounts(accounts []account) []account {
	kept := make([]account, 0, len(accounts))
	removed := 0
	for _, a := range accounts {
		if a.valid() {
			kept = append(kept, a)
		} else {
			removed++
		}
	}
	console.ClearScreen()
	printBanner()
	if removed == 0 {
		color.Info.Println("No expired accounts to remove.")
	} else {
		color.Success.Println(fmt.Sprintf("Removed %d expired account(s).", removed))
	}
	fmt.Println()
	return kept
}

func persistAccounts(accounts []account) error {
	out := make([]config.Account, 0, len(accounts))
	for _, a := range accounts {
		out = append(out, a.toConfig())
	}
	return config.SaveAccounts(out)
}

func printBanner() {
	fmt.Println(" __   __  _______  ______    _______  _______  ___      ___   _______  _______  ______   ")
	fmt.Println("|  | |  ||       ||    _ |  |  _    ||   _   ||   |    |   | |       ||       ||    _ |  ")
	fmt.Println("|  |_|  ||    ___||   | ||  | |_|   ||  |_|  ||   |    |   | |____   ||    ___||   | ||  ")
	fmt.Println("|       ||   |___ |   |_||_ |       ||       ||   |    |   |  ____|  ||   |___ |   |_||_ ")
	fmt.Println("|       ||    ___||    __  ||  _   | |       ||   |___ |   | | ______||    ___||    __  |")
	fmt.Println(" |     | |   |___ |   |  | || |_|   ||   _   ||       ||   | | |_____ |   |___ |   |  | |")
	fmt.Println("  |___|  |_______||___|  |_||_______||__| |__||_______||___| |_______||_______||___|  |_|")
	fmt.Println("Original by kartfr (https://github.com/kartFr)")
	fmt.Println("Improvements by rod (https://github.com/n3987frejhw098h324)")
	fmt.Println()
}

func printCookieInstructions() {
	fmt.Println("How to get your .ROBLOSECURITY:")
	fmt.Println()

	color.Info.Println("Method 1 - Browser extension")
	fmt.Println("  1. Install a cookie manager extension, e.g. \"Cookie-Editor\".")
	fmt.Println("  2. Open roblox.com, then open the extension on that page.")
	fmt.Println("  3. Find the cookie named .ROBLOSECURITY and copy its value.")
	fmt.Println()

	color.Info.Println("Method 2 - Browser dev tools (F12)")
	fmt.Println("  1. Go to roblox.com.")
	fmt.Println("  2. Press F12.")
	fmt.Println("  3. Open the \"Application\" tab (Chrome/Edge) or \"Storage\" tab (Firefox).")
	fmt.Println("  4. In the left panel click on \"Cookies\" > https://www.roblox.com.")
	fmt.Println("  5. Select .ROBLOSECURITY and copy its Value.")
	fmt.Println()
}

func getCookie(c *roblox.Client) error {
	var lastErr string
	for {
		console.ClearScreen()
		printBanner()
		printCookieInstructions()
		if lastErr != "" {
			color.Error.Println(lastErr)
			fmt.Println()
		}

		i, err := console.LongInput(".ROBLOSECURITY: ")
		if err != nil {
			if errors.Is(err, io.EOF) {
				return errInputClosed
			}
			lastErr = err.Error()
			continue
		}

		console.ClearScreen()
		printBanner()
		fmt.Println("Authenticating cookie...")
		if err := authenticateCookie(c, i); err != nil {
			lastErr = describeCookieError(err)
			continue
		}

		break
	}
	return nil
}

func authenticateCookie(c *roblox.Client, cookie string) error {
	err := c.SetCookie(cookie)
	if err == nil || isDefiniteCookieError(err) {
		return err
	}

	for _, delay := range []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second} {
		color.Warn.Println(fmt.Sprintf("Couldn't reach Roblox to verify the cookie - retrying in %s...", delay))
		time.Sleep(delay)

		err = c.SetCookie(cookie)
		if err == nil || isDefiniteCookieError(err) {
			return err
		}
	}
	return err
}

func isDefiniteCookieError(err error) bool {
	return errors.Is(err, roblox.ErrNoWarning) ||
		errors.Is(err, roblox.ErrCookieIncomplete) ||
		errors.Is(err, roblox.AuthenticateErrors.ErrAuthorizationDenied)
}

func describeCookieError(err error) string {
	switch {
	case errors.Is(err, roblox.ErrNoWarning):
		return "Copy the entire value, including the \"WARNING:-DO-NOT-SHARE-THIS\" text at the start."
	case errors.Is(err, roblox.ErrCookieIncomplete):
		return "The cookie looks cut off. Are you sure you copied the entire value?"
	case errors.Is(err, roblox.AuthenticateErrors.ErrAuthorizationDenied):
		return "That cookie is expired or invalid. Try a different cookie."
	default:
		return fmt.Sprintf("Couldn't verify the cookie with Roblox (%v). This is usually rate-limiting or a temporary outage, not a bad cookie. Try again in a minute.", err)
	}
}

func ensureAPIKey(c *roblox.Client) error {
	if strings.TrimSpace(c.APIKey) != "" {
		verbosef("API key already present for %s", c.UserInfo.Username)
		return nil
	}

	if c.Cookie != "" {
		fmt.Println("Creating API key for " + c.UserInfo.Username + "...")
		const apiKeyCreateTries = 3
		for attempt := 1; attempt <= apiKeyCreateTries; attempt++ {
			key, err := cloudauth.CreateAPIKey(c)
			if err == nil {
				if key = strings.TrimSpace(key); key != "" {
					c.APIKey = key
					verbosef("Created API key for %s", c.UserInfo.Username)
					return nil
				}
			} else {
				color.Error.Println(fmt.Sprintf("API key creation attempt %d/%d failed: %v", attempt, apiKeyCreateTries, err))
			}
			if attempt < apiKeyCreateTries {
				time.Sleep(2 * time.Second)
			}
		}
	}

	console.ClearScreen()
	printBanner()
	color.Warn.Println("Automatic API key creation failed for " + c.UserInfo.Username + ".")
	fmt.Println()
	fmt.Println("  1. Sign in as this account and go to https://create.roblox.com/dashboard/credentials")
	fmt.Println("  2. Create an API key with the asset:write permissions.")
	fmt.Println("  3. Paste the key below.")
	fmt.Println()
	for {
		key, err := console.Input("API key: ")
		if err != nil {
			if errors.Is(err, io.EOF) {
				return errInputClosed
			}
			color.Error.Println(err)
			continue
		}

		key = strings.TrimSpace(key)
		if key == "" {
			color.Error.Println("API key is required.")
			continue
		}

		c.APIKey = key
		break
	}
	return nil
}
