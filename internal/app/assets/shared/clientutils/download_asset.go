package clientutils

import (
	"bytes"

	"github.com/n3987frejhw098h324/verbalizerpublic/internal/roblox"
)

func FetchIcon(c *roblox.Client, getURL func() (string, error)) (*bytes.Buffer, error) {
	url, err := getURL()
	if err != nil {
		return nil, err
	}
	if url == "" {
		return nil, nil
	}
	return GetRequest(c, url)
}
