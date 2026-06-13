package badges

import (
	"bytes"
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

func TestNewCreateRequestFree(t *testing.T) {
	const universeID = 4242
	req, err := newCreateRequest(universeID, "First Win", "win a round", 0, bytes.NewBufferString("ICONPNG"), "image/png")
	if err != nil {
		t.Fatalf("newCreateRequest: %v", err)
	}

	wantURL := fmt.Sprintf(createURLFormat, universeID)
	if req.URL.String() != wantURL {
		t.Errorf("url = %q, want %q", req.URL.String(), wantURL)
	}

	fields, files := parseMultipart(t, req)
	if fields["request.name"] != "First Win" {
		t.Errorf("request.name = %q", fields["request.name"])
	}
	if fields["request.description"] != "win a round" {
		t.Errorf("request.description = %q", fields["request.description"])
	}
	if fields["request.paymentSourceType"] != "User" {
		t.Errorf("request.paymentSourceType = %q, want User", fields["request.paymentSourceType"])
	}
	if fields["request.expectedCost"] != "0" {
		t.Errorf("request.expectedCost = %q, want 0", fields["request.expectedCost"])
	}
	if fields["request.isActive"] != "true" {
		t.Errorf("request.isActive = %q, want true", fields["request.isActive"])
	}
	if got := string(files["request.files"]); got != "ICONPNG" {
		t.Errorf("request.files = %q, want ICONPNG", got)
	}
}

func TestNewCreateRequestPaidCost(t *testing.T) {
	req, err := newCreateRequest(1, "g", "g", 100, bytes.NewBufferString("x"), "image/png")
	if err != nil {
		t.Fatalf("newCreateRequest: %v", err)
	}
	fields, _ := parseMultipart(t, req)
	if fields["request.expectedCost"] != "100" {
		t.Errorf("request.expectedCost = %q, want 100", fields["request.expectedCost"])
	}
}

func TestIsInsufficientFundsMessage(t *testing.T) {
	cases := map[string]bool{
		"You have insufficient funds": true,
		"Not enough Robux":            true,
		"requires funds":              true,
		"everything is fine":          false,
		"name is moderated":           false,
	}
	for msg, want := range cases {
		if got := isInsufficientFundsMessage(msg); got != want {
			t.Errorf("isInsufficientFundsMessage(%q) = %v, want %v", msg, got, want)
		}
	}
}

func TestMessageOr(t *testing.T) {
	if got := messageOr([]byte(`{"errors":[{"message":"boom"}]}`), "fb"); got != "boom" {
		t.Errorf("got %q", got)
	}
	if got := messageOr(nil, "fb"); got != "fb" {
		t.Errorf("got %q, want fb", got)
	}
}
