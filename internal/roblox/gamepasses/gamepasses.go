package gamepasses

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
	detailsURLFormat = "https://apis.roblox.com/game-passes/v1/game-passes/%d/product-info"
	createURLFormat  = "https://apis.roblox.com/game-passes/v1/universes/%d/game-passes"
	updateURLFormat  = "https://apis.roblox.com/game-passes/v1/universes/%d/game-passes/%d"
)

var Errors = struct {
	ErrModerated        error
	ErrTokenInvalid     error
	ErrNotAuthenticated error
	ErrNotFound         error
	ErrWrongType        error
	ErrRateLimited      error
	ErrOffsale          error
}{
	ErrModerated:        errors.New("Roblox moderation flagged the game pass's name or description"),
	ErrTokenInvalid:     errors.New("Roblox rejected the request's security token (it will refresh and retry automatically)"),
	ErrNotAuthenticated: errors.New("you appear to be signed out - your Roblox cookie may have expired"),
	ErrNotFound:         errors.New("game pass not found (it may be deleted, private, or the id is not a game pass)"),
	ErrWrongType:        errors.New("id is not a game pass"),
	ErrRateLimited:      errors.New("rate limited"),
	ErrOffsale:          errors.New("game pass left off-sale (Roblox returned 204)"),
}

const productTypeGamePass = "Game Pass"

type Details struct {
	ID               int64
	Name             string
	Description      string
	Price            int64
	IsForSale        bool
	IconImageAssetID int64
	CreatorID        int64
}

type productInfoResponse struct {
	TargetID         int64  `json:"TargetId"`
	ProductType      string `json:"ProductType"`
	Name             string `json:"Name"`
	Description      string `json:"Description"`
	IconImageAssetID int64  `json:"IconImageAssetId"`
	PriceInRobux     *int64 `json:"PriceInRobux"`
	IsForSale        bool   `json:"IsForSale"`
	Creator          struct {
		CreatorTargetID int64 `json:"CreatorTargetId"`
	} `json:"Creator"`
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
		var info productInfoResponse
		if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
			return nil, err
		}
		if info.ProductType != productTypeGamePass {
			return nil, Errors.ErrWrongType
		}
		details := &Details{
			ID:               info.TargetID,
			Name:             info.Name,
			Description:      info.Description,
			IsForSale:        info.IsForSale,
			IconImageAssetID: info.IconImageAssetID,
			CreatorID:        info.Creator.CreatorTargetID,
		}
		if info.PriceInRobux != nil {
			details.Price = *info.PriceInRobux
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
	GamePassID int64 `json:"gamePassId"`
	ID         int64 `json:"id"`
}

func newCreateRequest(universeID int64, name, description string, icon *bytes.Buffer, contentType string) (*http.Request, error) {
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)
	formContentType := writer.FormDataContentType()

	go func() {
		defer pw.Close()

		fields := map[string]string{
			"name":        name,
			"description": description,
		}
		for key, value := range fields {
			if err := writer.WriteField(key, value); err != nil {
				_ = pw.CloseWithError(err)
				return
			}
		}

		if icon != nil {
			fileHeader := make(textproto.MIMEHeader)
			fileHeader.Set("Content-Disposition", `form-data; name="imageFile"; filename="icon"`)
			fileHeader.Set("Content-Type", contentType)
			part, err := writer.CreatePart(fileHeader)
			if err != nil {
				_ = pw.CloseWithError(err)
				return
			}
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

func Create(c *roblox.Client, universeID int64, name, description string, icon *bytes.Buffer, contentType string) (int64, error) {
	req, err := newCreateRequest(universeID, name, description, icon, contentType)
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
		if created.GamePassID != 0 {
			return created.GamePassID, nil
		}
		if created.ID != 0 {
			return created.ID, nil
		}
		return 0, errors.New("Roblox did not return a new game pass id")
	default:
		return 0, classifyWriteError(c, resp, body)
	}
}

func Update(c *roblox.Client, universeID, id int64, price int64, isForSale bool) error {
	fields := map[string]string{
		"isForSale":                strconv.FormatBool(isForSale),
		"isRegionalPricingEnabled": "false",
	}
	if isForSale {
		fields["price"] = strconv.FormatInt(price, 10)
	}

	body, contentType, err := multipartFields(fields)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("PATCH", fmt.Sprintf(updateURLFormat, universeID, id), body)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "RobloxStudio/WinInet")
	req.Header.Set("Content-Type", contentType)
	req.AddCookie(&http.Cookie{Name: ".ROBLOSECURITY", Value: c.Cookie})
	req.Header.Set("x-csrf-token", c.GetToken())

	resp, err := c.DoRequest(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil
	}
	if resp.StatusCode == http.StatusNoContent {
		return Errors.ErrOffsale
	}
	respBody, _ := io.ReadAll(resp.Body)
	return classifyWriteError(c, resp, respBody)
}

func multipartFields(fields map[string]string) (*bytes.Buffer, string, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return nil, "", err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return &buf, writer.FormDataContentType(), nil
}

func classifyWriteError(c *roblox.Client, resp *http.Response, body []byte) error {
	switch resp.StatusCode {
	case http.StatusForbidden:
		if token := resp.Header.Get("x-csrf-token"); token != "" {
			c.SetToken(token)
			return Errors.ErrTokenInvalid
		}
		return errors.New(messageOr(body, resp.Status))
	case http.StatusUnauthorized:
		return Errors.ErrNotAuthenticated
	case http.StatusNotFound:
		return Errors.ErrNotFound
	case http.StatusTooManyRequests:
		return Errors.ErrRateLimited
	case http.StatusBadRequest:
		message := messageOr(body, resp.Status)
		if isModeratedMessage(message) {
			return Errors.ErrModerated
		}
		return errors.New(message)
	default:
		return errors.New(messageOr(body, resp.Status))
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
