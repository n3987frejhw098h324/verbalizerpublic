package thumbnails

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/n3987frejhw098h324/verbalizerpublic/internal/retry"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox"
)

var ErrRateLimited = errors.New("rate limited")

const (
	gamePassURLFormat         = "https://thumbnails.roblox.com/v1/game-passes?gamePassIds=%d&size=150x150&format=Png&isCircular=false"
	badgeURLFormat            = "https://thumbnails.roblox.com/v1/badges/icons?badgeIds=%d&size=150x150&format=Png&isCircular=false"
	developerProductURLFormat = "https://thumbnails.roblox.com/v1/developer-products/icons?developerProductIds=%d&size=150x150&format=Png&isCircular=false"
)

type thumbnailResponse struct {
	Data []struct {
		TargetID int64  `json:"targetId"`
		State    string `json:"state"`
		ImageURL string `json:"imageUrl"`
	} `json:"data"`
}

func imageURLFrom(parsed thumbnailResponse) string {
	if len(parsed.Data) == 0 || parsed.Data[0].State != "Completed" {
		return ""
	}
	return parsed.Data[0].ImageURL
}

func GamePassIcon(c *roblox.Client, gamePassID int64) (string, error) {
	return fetch(c, fmt.Sprintf(gamePassURLFormat, gamePassID))
}

func BadgeIcon(c *roblox.Client, badgeID int64) (string, error) {
	return fetch(c, fmt.Sprintf(badgeURLFormat, badgeID))
}

func DeveloperProductIcon(c *roblox.Client, targetID int64) (string, error) {
	return fetch(c, fmt.Sprintf(developerProductURLFormat, targetID))
}

func fetch(c *roblox.Client, url string) (string, error) {
	return retry.Do(
		retry.NewOptions(retry.Tries(3), retry.Delay(time.Second)),
		func(_ int) (string, error) {
			req, err := http.NewRequest("GET", url, http.NoBody)
			if err != nil {
				return "", &retry.ExitRetry{Err: err}
			}
			req.AddCookie(&http.Cookie{Name: ".ROBLOSECURITY", Value: c.Cookie})

			resp, err := c.DoRequest(req)
			if err != nil {
				return "", &retry.ContinueRetry{Err: err}
			}
			defer resp.Body.Close()

			switch resp.StatusCode {
			case http.StatusOK:
				var parsed thumbnailResponse
				if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
					return "", &retry.ExitRetry{Err: err}
				}
				return imageURLFrom(parsed), nil
			case http.StatusTooManyRequests:
				return "", &retry.ContinueRetry{Err: ErrRateLimited}
			default:
				return "", &retry.ExitRetry{Err: errors.New(resp.Status)}
			}
		},
	)
}
