package developerproducts

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

func TestNewCreateRequest(t *testing.T) {
	const universeID = 777
	req, err := newCreateRequest(universeID, "Coins", "100 coins", bytes.NewBufferString("ICON"), "image/png")
	if err != nil {
		t.Fatalf("newCreateRequest: %v", err)
	}

	wantURL := fmt.Sprintf(createURLFormat, universeID)
	if req.URL.String() != wantURL {
		t.Errorf("url = %q, want %q", req.URL.String(), wantURL)
	}

	fields, files := parseMultipart(t, req)
	if fields["name"] != "Coins" {
		t.Errorf("name = %q", fields["name"])
	}
	if fields["description"] != "100 coins" {
		t.Errorf("description = %q", fields["description"])
	}
	if _, ok := fields["price"]; ok {
		t.Error("price should be set via the PATCH update, not at create")
	}
	if got := string(files["imageFile"]); got != "ICON" {
		t.Errorf("imageFile = %q, want ICON", got)
	}
}

func TestNewCreateRequestNoIcon(t *testing.T) {
	req, err := newCreateRequest(1, "Coins", "", nil, "")
	if err != nil {
		t.Fatalf("newCreateRequest: %v", err)
	}
	_, files := parseMultipart(t, req)
	if _, ok := files["imageFile"]; ok {
		t.Error("expected no imageFile part when icon is nil")
	}
}

func TestDetailsIdentifiesDeveloperProduct(t *testing.T) {
	body := `{"DisplayName":"Mid Grade Crate","UniverseId":1176784616,"TargetId":70619997,"ProductType":"Developer Product","ProductId":3413305154,"Name":"Mid Grade Crate","Description":"","PriceInRobux":25,"IsForSale":true}`
	var info detailsResponse
	if err := json.Unmarshal([]byte(body), &info); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if info.ProductType != productTypeDeveloperProduct {
		t.Errorf("ProductType = %q, want %q", info.ProductType, productTypeDeveloperProduct)
	}
	if info.ProductID != 3413305154 {
		t.Errorf("ProductID = %d, want 3413305154", info.ProductID)
	}
	if info.TargetID != 70619997 {
		t.Errorf("TargetID = %d, want 70619997", info.TargetID)
	}
}

func TestCreateResponseProductID(t *testing.T) {
	body := `{"productId":3601852660,"name":"eafzdxf","description":"dfa","iconImageAssetId":89274379797597,"universeId":10243451107,"isForSale":false,"storePageEnabled":false}`
	var created createResponse
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if created.ProductID != 3601852660 {
		t.Errorf("ProductID = %d, want 3601852660", created.ProductID)
	}
}

func TestMessageOr(t *testing.T) {
	if got := messageOr([]byte(`{"errors":[{"message":"nope"}]}`), "fb"); got != "nope" {
		t.Errorf("got %q", got)
	}
	if got := messageOr([]byte("garbage"), "fb"); got != "fb" {
		t.Errorf("got %q, want fb", got)
	}
}
