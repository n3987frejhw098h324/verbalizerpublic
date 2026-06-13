package developerproducts

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
	detailsURLFormat = "https://apis.roblox.com/developer-products/v1/developer-products/%d/details"
	createURLFormat  = "https://apis.roblox.com/developer-products/v2/universes/%d/developer-products"
	updateURLFormat  = "https://apis.roblox.com/developer-products/v2/universes/%d/developer-products/%d"
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
	ErrModerated:        errors.New("Roblox moderation flagged the developer product's name or description"),
	ErrTokenInvalid:     errors.New("Roblox rejected the request's security token (it will refresh and retry automatically)"),
	ErrNotAuthenticated: errors.New("you appear to be signed out - your Roblox cookie may have expired"),
	ErrNotFound:         errors.New("developer product not found (it may be deleted or the id is not a developer product)"),
	ErrWrongType:        errors.New("id is not a developer product"),
	ErrRateLimited:      errors.New("rate limited"),
	ErrOffsale:          errors.New("developer product left off-sale (Roblox returned 204)"),
}

const productTypeDeveloperProduct = "Developer Product"

type Details struct {
	ProductID   int64
	TargetID    int64
	Name        string
	Description string
	Price       int64
}

type detailsResponse struct {
	ProductID    int64  `json:"ProductId"`
	TargetID     int64  `json:"TargetId"`
	ProductType  string `json:"ProductType"`
	Name         string `json:"Name"`
	Description  string `json:"Description"`
	PriceInRobux *int64 `json:"PriceInRobux"`
}

func GetDetails(c *roblox.Client, productID int64) (*Details, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf(detailsURLFormat, productID), http.NoBody)
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
		if info.ProductType != productTypeDeveloperProduct {
			return nil, Errors.ErrWrongType
		}
		details := &Details{
			ProductID:   info.ProductID,
			TargetID:    info.TargetID,
			Name:        info.Name,
			Description: info.Description,
		}
		if details.ProductID == 0 {
			details.ProductID = productID
		}
		if info.PriceInRobux != nil {
			details.Price = *info.PriceInRobux
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
	ID        int64 `json:"id"`
	ProductID int64 `json:"productId"`
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
		if created.ProductID != 0 {
			return created.ProductID, nil
		}
		if created.ID != 0 {
			return created.ID, nil
		}
		return 0, errors.New("Roblox did not return a new developer product id")
	default:
		return 0, classifyWriteError(c, resp, body)
	}
}

func Update(c *roblox.Client, universeID, id int64, name, description string, price int64, isForSale bool) error {
	fields := map[string]string{
		"name":                     name,
		"description":              description,
		"isForSale":                strconv.FormatBool(isForSale),
		"isRegionalPricingEnabled": "false",
		"storePageEnabled":         "false",
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
