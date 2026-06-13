package assets

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox"
)

var UpdatePermissionErrors = struct {
	ErrTokenInvalid     error
	ErrNotAuthenticated error
}{
	ErrTokenInvalid:     errors.New("Roblox rejected the request's security token (it will refresh and retry automatically)"),
	ErrNotAuthenticated: errors.New("you appear to be signed out - your Roblox cookie may have expired"),
}

type PermissionRequestItem struct {
	SubjectType string `json:"subjectType"`
	SubjectID   string `json:"subjectId"`
	Action      string `json:"action"`
}

type PermissionRequest struct {
	Requests []PermissionRequestItem `json:"requests"`
}

type PermissionResponse struct {
	Errors []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors,omitempty"`
}

func newUpdatePermissionsRequest(assetID int64, body PermissionRequest) (*http.Request, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://apis.roblox.com/asset-permissions-api/v1/assets/%d/permissions", assetID)
	req, err := http.NewRequest("PATCH", url, bytes.NewReader(jsonBody))
	if err != nil {
		return req, err
	}
	req.Header.Set("Content-Type", "application/json")

	return req, nil
}

func NewUpdatePermissionsHandler(c *roblox.Client, assetID int64, body PermissionRequest) (func() (*PermissionResponse, error), error) {
	if _, err := newUpdatePermissionsRequest(assetID, body); err != nil {
		return func() (*PermissionResponse, error) { return nil, nil }, err
	}

	return func() (*PermissionResponse, error) {
		req, err := newUpdatePermissionsRequest(assetID, body)
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

		var response PermissionResponse
		decodeErr := json.NewDecoder(resp.Body).Decode(&response)

		switch resp.StatusCode {
		case http.StatusOK:
			if decodeErr != nil {
				return nil, decodeErr
			}
			return &response, nil
		case http.StatusUnauthorized:
			return nil, UpdatePermissionErrors.ErrNotAuthenticated
		case http.StatusForbidden:
			c.SetToken(resp.Header.Get("x-csrf-token"))
			return nil, UpdatePermissionErrors.ErrTokenInvalid
		default:
			if len(response.Errors) > 0 {
				if message := response.Errors[0].Message; message != "" {
					return nil, errors.New(response.Errors[0].Message)
				}
			}

			return nil, errors.New(resp.Status)
		}
	}, nil
}
