package games

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/n3987frejhw098h324/verbalizerpublic/internal/retry"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox"
)

var ErrRateLimited = errors.New("rate limited")

func NewGroupGamesHandler(c *roblox.Client, groupID int64) (func() (*GamesResponse, error), error) {
	url := fmt.Sprintf("https://games.roblox.com/v2/groups/%d/gamesV2?limit=100", groupID)
	req, err := http.NewRequest("GET", url, http.NoBody)
	if err != nil {
		return func() (*GamesResponse, error) { return nil, nil }, err
	}

	return func() (*GamesResponse, error) {
		req.AddCookie(&http.Cookie{
			Name:  ".ROBLOSECURITY",
			Value: c.Cookie,
		})

		resp, err := c.DoRequest(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests {
			return nil, ErrRateLimited
		}
		if resp.StatusCode != http.StatusOK {
			return nil, errors.New(resp.Status)
		}

		var gamesResponse GamesResponse
		if err := json.NewDecoder(resp.Body).Decode(&gamesResponse); err != nil {
			return nil, err
		}
		return &gamesResponse, nil
	}, nil
}

func GroupGames(c *roblox.Client, groupID int64) (*GamesResponse, error) {
	handler, err := NewGroupGamesHandler(c, groupID)
	if err != nil {
		return nil, err
	}

	return retry.Do(
		retry.NewOptions(retry.Tries(3)),
		func(_ int) (*GamesResponse, error) {
			placeDetails, err := handler()
			if err != nil {
				if errors.Is(err, ErrRateLimited) {
					return nil, &retry.ExitRetry{Err: err}
				}
				return nil, &retry.ContinueRetry{Err: err}
			}

			return placeDetails, nil
		},
	)
}
