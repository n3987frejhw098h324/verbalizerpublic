package badges

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"

	"github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox"
)

const (
	detailsURLFormat = "https://badges.roblox.com/v1/badges/%d"
	createURLFormat  = "https://badges.roblox.com/v1/universes/%d/badges"
)

var Errors = struct {
	ErrModerated         error
	ErrTokenInvalid      error
	ErrNotAuthenticated  error
	ErrNotFound          error
	ErrRateLimited       error
	ErrInsufficientFunds error
}{
	ErrModerated:         errors.New("Roblox moderation flagged the badge's name or description"),
	ErrTokenInvalid:      errors.New("Roblox rejected the request's security token (it will refresh and retry automatically)"),
	ErrNotAuthenticated:  errors.New("you appear to be signed out - your Roblox cookie may have expired"),
	ErrNotFound:          errors.New("badge not found (it may be deleted or the id is not a badge)"),
	ErrRateLimited:       errors.New("rate limited"),
	ErrInsufficientFunds: errors.New("not enough Robux to create this badge (5 badges per game are free each day, then 100 Robux each)"),
}

type Details struct {
	ID                 int64
	Name               string
	Description        string
	IconImageID        int64
	AwardingUniverseID int64
}

type detailsResponse struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	Description        string `json:"description"`
	DisplayIconImageID int64  `json:"displayIconImageId"`
	IconImageID        int64  `json:"iconImageId"`
	AwardingUniverse   struct {
		ID int64 `json:"id"`
	} `json:"awardingUniverse"`
}

func GetDetails(c *roblox.Client, id int64) (*Details, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf(detailsURLFormat, id), http.NoBody)
	if err != nil {
		return nil, err
	}
	req.AddCookie(&http.Cookie{Name: ".ROBLOSECURITY", Value: c.Cookie})

	resp, err := c.DoRequest(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var info detailsResponse
		if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
			return nil, err
		}
		iconID := info.IconImageID
		if iconID == 0 {
			iconID = info.DisplayIconImageID
		}
		details := &Details{
			ID:                 info.ID,
			Name:               info.Name,
			Description:        info.Description,
			IconImageID:        iconID,
			AwardingUniverseID: info.AwardingUniverse.ID,
		}
		if details.ID == 0 {
			details.ID = id
		}
		return details, nil
	case http.StatusNotFound:
		return nil, Errors.ErrNotFound
	case http.StatusUnauthorized:
		return nil, Errors.ErrNotAuthenticated
	case http.StatusTooManyRequests:
		return nil, Errors.ErrRateLimited
	default:
		return nil, errors.New(resp.Status)
	}
}

type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type createResponse struct {
	ID int64 `json:"id"`
}

func newCreateRequest(universeID int64, name, description string, expectedCost int64, icon *bytes.Buffer, contentType string) (*http.Request, error) {
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)
	formContentType := writer.FormDataContentType()

	go func() {
		defer pw.Close()

		fields := map[string]string{
			"request.name":              name,
			"request.description":       description,
			"request.paymentSourceType": "User",
			"request.expectedCost":      strconv.FormatInt(expectedCost, 10),
			"request.isActive":          "true",
		}
		for key, value := range fields {
			if err := writer.WriteField(key, value); err != nil {
				_ = pw.CloseWithError(err)
				return
			}
		}

		fileHeader := make(textproto.MIMEHeader)
		fileHeader.Set("Content-Disposition", `form-data; name="request.files"; filename="icon"`)
		fileHeader.Set("Content-Type", contentType)
		part, err := writer.CreatePart(fileHeader)
		if err != nil {
			_ = pw.CloseWithError(err)
			return
		}
		if icon != nil {
			if _, err := io.Copy(part, bytes.NewReader(icon.Bytes())); err != nil {
				_ = pw.CloseWithError(err)
				return
			}
		}

		if err := writer.Close(); err != nil {
			_ = pw.CloseWithError(err)
		}
	}()

	req, err := http.NewRequest("POST", fmt.Sprintf(createURLFormat, universeID), pr)
	if err != nil {
		_ = pr.Close()
		return nil, err
	}
	req.Header.Set("User-Agent", "RobloxStudio/WinInet")
	req.Header.Set("Content-Type", formContentType)
	return req, nil
}

func Create(c *roblox.Client, universeID int64, name, description string, expectedCost int64, icon *bytes.Buffer, contentType string) (int64, error) {
	req, err := newCreateRequest(universeID, name, description, expectedCost, icon, contentType)
	if err != nil {
		return 0, err
	}
	req.AddCookie(&http.Cookie{Name: ".ROBLOSECURITY", Value: c.Cookie})
	req.Header.Set("x-csrf-token", c.GetToken())

	resp, err := c.DoRequest(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	switch resp.StatusCode {
	case http.StatusOK:
		var created createResponse
		if err := json.Unmarshal(body, &created); err != nil {
			return 0, err
		}
		if created.ID == 0 {
			return 0, errors.New("Roblox did not return a new badge id")
		}
		return created.ID, nil
	case http.StatusForbidden:
		if token := resp.Header.Get("x-csrf-token"); token != "" {
			c.SetToken(token)
			return 0, Errors.ErrTokenInvalid
		}
		return 0, errors.New(messageOr(body, resp.Status))
	case http.StatusUnauthorized:
		return 0, Errors.ErrNotAuthenticated
	case http.StatusTooManyRequests:
		return 0, Errors.ErrRateLimited
	case http.StatusBadRequest:
		message := messageOr(body, resp.Status)
		switch {
		case isInsufficientFundsMessage(message):
			return 0, Errors.ErrInsufficientFunds
		case isModeratedMessage(message):
			return 0, Errors.ErrModerated
		}
		return 0, errors.New(message)
	default:
		return 0, errors.New(messageOr(body, resp.Status))
	}
}

func messageOr(body []byte, fallback string) string {
	var parsed struct {
		Errors  []apiError `json:"errors"`
		Message string     `json:"message"`
	}
	if err := json.Unmarshal(body, &parsed); err == nil {
		if len(parsed.Errors) > 0 && parsed.Errors[0].Message != "" {
			return parsed.Errors[0].Message
		}
		if parsed.Message != "" {
			return parsed.Message
		}
	}
	return fallback
}

func isModeratedMessage(message string) bool {
	lowered := bytes.ToLower([]byte(message))
	return bytes.Contains(lowered, []byte("moderat")) || bytes.Contains(lowered, []byte("inappropriate"))
}

func isInsufficientFundsMessage(message string) bool {
	lowered := bytes.ToLower([]byte(message))
	return bytes.Contains(lowered, []byte("insufficient")) ||
		bytes.Contains(lowered, []byte("not enough")) ||
		bytes.Contains(lowered, []byte("funds")) ||
		bytes.Contains(lowered, []byte("robux to"))
}
