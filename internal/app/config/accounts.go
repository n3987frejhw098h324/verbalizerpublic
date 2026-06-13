package config

import (
	"encoding/json"
	"strings"
)

type Account struct {
	Cookie string `json:"cookie"`
	APIKey string `json:"apiKey"`
	UserID int64  `json:"userId,omitempty"`
}

const accountsKey = "accounts"

func LoadAccounts() []Account {
	if raw := strings.TrimSpace(config[accountsKey]); raw != "" {
		var accounts []Account
		if err := json.Unmarshal([]byte(raw), &accounts); err == nil {
			return cleanAccounts(accounts)
		}
	}

	legacy := Account{
		Cookie: strings.TrimSpace(config["cookie"]),
		APIKey: strings.TrimSpace(config["api_key"]),
	}
	return cleanAccounts([]Account{legacy})
}

func cleanAccounts(accounts []Account) []Account {
	out := make([]Account, 0, len(accounts))
	seen := make(map[string]struct{}, len(accounts))
	for _, a := range accounts {
		a.Cookie = strings.TrimSpace(a.Cookie)
		a.APIKey = strings.TrimSpace(a.APIKey)
		if a.Cookie == "" {
			continue
		}
		if _, dup := seen[a.Cookie]; dup {
			continue
		}
		seen[a.Cookie] = struct{}{}
		out = append(out, a)
	}
	return out
}

func SaveAccounts(accounts []Account) error {
	accounts = cleanAccounts(accounts)

	data, err := json.Marshal(accounts)
	if err != nil {
		return err
	}
	config[accountsKey] = string(data)

	config["cookie"] = ""
	config["api_key"] = ""
	if len(accounts) > 0 {
		config["cookie"] = accounts[0].Cookie
		config["api_key"] = accounts[0].APIKey
	}

	return Save()
}
