package gamepasses

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"testing"
)

func parseMultipart(t *testing.T, req *http.Request) (map[string]string, map[string][]byte) {
	t.Helper()

	mediaType, params, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("parsing Content-Type: %v", err)
	}
	if mediaType != "multipart/form-data" {
		t.Fatalf("Content-Type = %q, want multipart/form-data", mediaType)
	}

	reader := multipart.NewReader(req.Body, params["boundary"])
	fields := map[string]string{}
	files := map[string][]byte{}
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("reading multipart part: %v", err)
		}
		data, err := io.ReadAll(part)
		if err != nil {
			t.Fatalf("reading part body: %v", err)
		}
		if part.FileName() != "" {
			files[part.FormName()] = data
		} else {
			fields[part.FormName()] = string(data)
		}
	}
	return fields, files
}

func TestNewCreateRequestWithIcon(t *testing.T) {
	const universeID = 12345
	req, err := newCreateRequest(universeID, "My Pass", "a description", bytes.NewBufferString("ICONBYTES"), "image/png")
	if err != nil {
		t.Fatalf("newCreateRequest: %v", err)
	}

	if req.Method != http.MethodPost {
		t.Errorf("method = %q, want POST", req.Method)
	}
	wantURL := fmt.Sprintf(createURLFormat, universeID)
	if req.URL.String() != wantURL {
		t.Errorf("url = %q, want %q", req.URL.String(), wantURL)
	}

	fields, files := parseMultipart(t, req)
	if fields["name"] != "My Pass" {
		t.Errorf("name = %q, want %q", fields["name"], "My Pass")
	}
	if fields["description"] != "a description" {
		t.Errorf("description = %q", fields["description"])
	}
	if _, ok := fields["UniverseId"]; ok {
		t.Error("universe id should be in the URL path, not a form field")
	}
	if got := string(files["imageFile"]); got != "ICONBYTES" {
		t.Errorf("imageFile = %q, want ICONBYTES", got)
	}
}

func TestNewCreateRequestNoIcon(t *testing.T) {
	req, err := newCreateRequest(1, "Pass", "", nil, "")
	if err != nil {
		t.Fatalf("newCreateRequest: %v", err)
	}
	_, files := parseMultipart(t, req)
	if _, ok := files["imageFile"]; ok {
		t.Error("expected no imageFile part when icon is nil")
	}
}

func TestIsModeratedMessage(t *testing.T) {
	cases := map[string]bool{
		"Name is moderated":            true,
		"inappropriate name":           true,
		"Something went wrong":         false,
		"INAPPROPRIATE NAME OR DESC":   true,
		"price must be greater than 0": false,
	}
	for msg, want := range cases {
		if got := isModeratedMessage(msg); got != want {
			t.Errorf("isModeratedMessage(%q) = %v, want %v", msg, got, want)
		}
	}
}

func TestProductInfoIdentifiesGamePass(t *testing.T) {
	body := `{"TargetId":13534630,"ProductType":"Game Pass","Name":"Toxic Gunner","Description":"d","IconImageAssetId":104393367124137,"PriceInRobux":699,"IsForSale":true,"Creator":{"CreatorTargetId":4914494}}`
	var info productInfoResponse
	if err := json.Unmarshal([]byte(body), &info); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if info.ProductType != productTypeGamePass {
		t.Errorf("ProductType = %q, want %q", info.ProductType, productTypeGamePass)
	}
	if info.TargetID != 13534630 {
		t.Errorf("TargetID = %d", info.TargetID)
	}
}

func TestMessageOr(t *testing.T) {
	if got := messageOr([]byte(`{"errors":[{"code":1,"message":"bad name"}]}`), "fallback"); got != "bad name" {
		t.Errorf("got %q, want bad name", got)
	}
	if got := messageOr([]byte(`{"message":"top level"}`), "fallback"); got != "top level" {
		t.Errorf("got %q, want top level", got)
	}
	if got := messageOr([]byte("not json"), "fallback"); got != "fallback" {
		t.Errorf("got %q, want fallback", got)
	}
}
