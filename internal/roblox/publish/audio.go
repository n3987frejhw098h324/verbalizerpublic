package publish

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox"
)

var UploadAudioErrors = struct {
	ErrModerated        error
	ErrAccountModerated error
	ErrTokenInvalid     error
	ErrNotAuthenticated error
	ErrQuotaExceeded    error
}{
	ErrModerated:        errors.New("Roblox moderation flagged the audio's name or description"),
	ErrAccountModerated: errors.New("your Roblox account has been moderated"),
	ErrTokenInvalid:     errors.New("Roblox rejected the request's security token (it will refresh and retry automatically)"),
	ErrNotAuthenticated: errors.New("you appear to be signed out - your Roblox cookie may have expired"),
	ErrQuotaExceeded:    errors.New("you have reached Roblox's audio upload limit for your account"),
}

var ErrRateLimited = errors.New("rate limited")

type uploadAudioRequest struct {
	Name              string  `json:"name"`
	File              string  `json:"file"`
	GroupID           int64   `json:"groupId,omitempty"`
	PaymentSource     string  `json:"paymentSource,omitempty"`
	EstimatedFileSize int64   `json:"estimatedFileSize"`
	EstimatedDuration float64 `json:"estimatedDuration"`
	AssetPrivacy      int32   `json:"assetPrivacy"`
}

type publishAudioResponse struct {
	ID     int64  `json:"id,omitempty"`
	Name   string `json:"name,omitempty"`
	Errors []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors,omitempty"`
}

func newUploadAudioRequest(name string, data *bytes.Buffer, groupID ...int64) (*http.Request, error) {
	var buffer bytes.Buffer
	encoder := base64.NewEncoder(base64.StdEncoding, &buffer)
	size := int64(data.Len())
	if _, err := io.Copy(encoder, bytes.NewReader(data.Bytes())); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}

	body := uploadAudioRequest{
		Name:              name,
		File:              buffer.String(),
		EstimatedFileSize: size,
	}
	if len(groupID) > 0 {
		body.GroupID = groupID[0]
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", "https://publish.roblox.com/v1/audio", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "RobloxStudio/WinInet")
	req.Header.Set("Content-Type", "application/json")

	return req, nil
}

func NewUploadAudioHandler(c *roblox.Client, name string, data *bytes.Buffer, groupID ...int64) (func() (*publishAudioResponse, error), error) {
	if _, err := newUploadAudioRequest(name, data, groupID...); err != nil {
		return func() (*publishAudioResponse, error) { return nil, nil }, err
	}

	currentName := name
	return func() (*publishAudioResponse, error) {
		req, err := newUploadAudioRequest(currentName, data, groupID...)
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

		var response publishAudioResponse
		decodeErr := json.NewDecoder(resp.Body).Decode(&response)

		if decodeErr == nil && len(response.Errors) > 0 && response.Errors[0].Message == "User is moderated" {
			return nil, UploadAudioErrors.ErrAccountModerated
		}

		switch resp.StatusCode {
		case http.StatusOK:
			if decodeErr != nil {
				return nil, decodeErr
			}
			return &response, nil
		case http.StatusBadRequest:
			if len(response.Errors) == 0 {
				return nil, errors.New(resp.Status)
			}

			message := response.Errors[0].Message
			if message == "Audio name or description is moderated." {
				currentName = "[Censored]"
				return nil, UploadAudioErrors.ErrModerated
			}

			return nil, errors.New(message)
		case http.StatusUnauthorized:
			if len(response.Errors) == 0 {
				return nil, errors.New(resp.Status)
			}

			message := response.Errors[0].Message
			if message == "User is not authenticated" {
				return nil, UploadAudioErrors.ErrNotAuthenticated
			}

			return nil, errors.New(message)
		case http.StatusForbidden:
			c.SetToken(resp.Header.Get("x-csrf-token"))
			return nil, UploadAudioErrors.ErrTokenInvalid
		case http.StatusTooManyRequests:
			if len(response.Errors) > 0 && response.Errors[0].Message == "Audio upload has exceeded user's quota." {
				return nil, UploadAudioErrors.ErrQuotaExceeded
			}

			return nil, ErrRateLimited
		default:
			if len(response.Errors) == 0 {
				return nil, errors.New(resp.Status)
			}

			return nil, errors.New(response.Errors[0].Message)
		}
	}, nil
}
