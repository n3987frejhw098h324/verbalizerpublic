package thumbnails

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestURLFormats(t *testing.T) {
	cases := []struct {
		got  string
		want string
	}{
		{fmt.Sprintf(gamePassURLFormat, 12408696), "https://thumbnails.roblox.com/v1/game-passes?gamePassIds=12408696&size=150x150&format=Png&isCircular=false"},
		{fmt.Sprintf(badgeURLFormat, 2145915792), "https://thumbnails.roblox.com/v1/badges/icons?badgeIds=2145915792&size=150x150&format=Png&isCircular=false"},
		{fmt.Sprintf(developerProductURLFormat, 70619997), "https://thumbnails.roblox.com/v1/developer-products/icons?developerProductIds=70619997&size=150x150&format=Png&isCircular=false"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("url = %q, want %q", c.got, c.want)
		}
	}
}

func TestImageURLFrom(t *testing.T) {
	const completed = `{"data":[{"targetId":2145915792,"state":"Completed","imageUrl":"https://tr.rbxcdn.com/x/150/150/Image/Png/noFilter","version":"TN3"}]}`
	var parsed thumbnailResponse
	if err := json.Unmarshal([]byte(completed), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := imageURLFrom(parsed); got != "https://tr.rbxcdn.com/x/150/150/Image/Png/noFilter" {
		t.Errorf("imageURLFrom = %q", got)
	}

	for _, body := range []string{
		`{"data":[]}`,
		`{"data":[{"targetId":1,"state":"Pending","imageUrl":""}]}`,
		`{"data":[{"targetId":1,"state":"Blocked","imageUrl":"x"}]}`,
	} {
		var p thumbnailResponse
		if err := json.Unmarshal([]byte(body), &p); err != nil {
			t.Fatalf("unmarshal %q: %v", body, err)
		}
		if got := imageURLFrom(p); got != "" {
			t.Errorf("imageURLFrom(%s) = %q, want empty", body, got)
		}
	}
}
