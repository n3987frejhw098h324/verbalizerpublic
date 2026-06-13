package cloudauth

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/n3987frejhw098h324/verbalizerpublic/internal/retry"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox"
)

const createAPIKeyURL = "https://apis.roblox.com/cloud-authentication/v1/apiKey"

var CreateAPIKeyErrors = struct {
	ErrTokenInvalid     error
	ErrNotAuthenticated error
	ErrNoSecret         error
}{
	ErrTokenInvalid:     errors.New("Roblox rejected the request's security token (it will refresh and retry automatically)"),
	ErrNotAuthenticated: errors.New("you appear to be signed out - your Roblox cookie may have expired"),
	ErrNoSecret:         errors.New("Roblox's response did not include an API key - you may need to create one manually"),
}

type scope struct {
	ScopeType   string   `json:"scopeType"`
	TargetParts []string `json:"targetParts"`
	Operations  []string `json:"operations"`
}

type userConfiguredProperties struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	IsEnabled    bool     `json:"isEnabled"`
	AllowedCidrs []string `json:"allowedCidrs"`
	Scopes       []scope  `json:"scopes"`
}

type createAPIKeyRequest struct {
	CloudAuthUserConfiguredProperties userConfiguredProperties `json:"cloudAuthUserConfiguredProperties"`
}

type createAPIKeyResponse struct {
	APIKeySecret string `json:"apikeySecret"`
	Errors       []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors,omitempty"`
}

func randomName() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "Verbalizer"
	}
	return "Verbalizer-" + hex.EncodeToString(b)
}

func newCreateAPIKeyRequest() (*http.Request, error) {
	body := createAPIKeyRequest{
		CloudAuthUserConfiguredProperties: userConfiguredProperties{
			Name:         randomName(),
			Description:  "",
			IsEnabled:    true,
			AllowedCidrs: []string{"0.0.0.0/0"},
			Scopes: []scope{
				{
					ScopeType:   "asset",
					TargetParts: []string{"*"},
					Operations:  []string{"write"},
				},
			},
		},
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", createAPIKeyURL, bytes.NewReader(jsonBody))
	if err != nil {
		return req, err
	}
	req.Header.Set("Content-Type", "application/json")

	return req, nil
}

func newCreateAPIKeyHandler(c *roblox.Client) (func() (*createAPIKeyResponse, error), error) {
	if _, err := newCreateAPIKeyRequest(); err != nil {
		return func() (*createAPIKeyResponse, error) { return nil, nil }, err
	}

	return func() (*createAPIKeyResponse, error) {
		req, err := newCreateAPIKeyRequest()
		if err != nil {
			return nil, err
		}
		req.AddCookie(&http.Cookie{
			Name:  ".ROBLOSECURITY",
			Value: c.Cookie,
		})
		req.Header.Set("x-csrf-token", c.GetToken())

		resp, err := c.DoRequest(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		var response createAPIKeyResponse
		decodeErr := json.NewDecoder(resp.Body).Decode(&response)

		switch resp.StatusCode {
		case http.StatusOK, http.StatusCreated:
			if decodeErr != nil {
				return nil, decodeErr
			}
			if response.APIKeySecret == "" {
				return nil, CreateAPIKeyErrors.ErrNoSecret
			}
			return &response, nil
		case http.StatusUnauthorized:
			return nil, CreateAPIKeyErrors.ErrNotAuthenticated
		case http.StatusForbidden:
			c.SetToken(resp.Header.Get("x-csrf-token"))
			return nil, CreateAPIKeyErrors.ErrTokenInvalid
		default:
			if len(response.Errors) > 0 {
				if message := response.Errors[0].Message; message != "" {
					return nil, errors.New(message)
				}
			}
			return nil, errors.New(resp.Status)
		}
	}, nil
}

func CreateAPIKey(c *roblox.Client) (string, error) {
	handler, err := newCreateAPIKeyHandler(c)
	if err != nil {
		return "", err
	}

	response, err := retry.Do(
		retry.NewOptions(retry.Tries(3)),
		func(_ int) (*createAPIKeyResponse, error) {
			res, err := handler()
			if err != nil {
				switch err {
				case CreateAPIKeyErrors.ErrTokenInvalid:
					return nil, &retry.ContinueRetry{Err: err}
				case CreateAPIKeyErrors.ErrNotAuthenticated:
					return nil, &retry.ExitRetry{Err: err}
				default:
					return nil, &retry.ContinueRetry{Err: err}
				}
			}
			return res, nil
		},
	)
	if err != nil {
		return "", err
	}

	return response.APIKeySecret, nil
}
