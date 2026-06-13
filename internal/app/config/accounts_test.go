package config

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

func reparseConfigFile(t *testing.T) {
	t.Helper()
	for k := range config {
		delete(config, k)
	}
	data, err := os.ReadFile("config.ini")
	if err != nil {
		t.Fatalf("read config.ini: %v", err)
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		split := strings.SplitN(line, "=", 2)
		if len(split) != 2 {
			continue
		}
		key := strings.TrimSpace(split[0])
		if key == "" {
			continue
		}
		config[key] = split[1]
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan config.ini: %v", err)
	}
}

func TestSaveAndLoadAccountsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)

	in := []Account{
		{Cookie: "_|WARNING:-DO-NOT-SHARE-THIS.--Sharing-this.|_AAAbbb==", APIKey: "key-one==", UserID: 111},
		{Cookie: "_|WARNING:-DO-NOT-SHARE-THIS.--Sharing-this.|_CCCddd/+", APIKey: "key-two", UserID: 222},
	}
	if err := SaveAccounts(in); err != nil {
		t.Fatalf("save: %v", err)
	}

	reparseConfigFile(t)

	got := LoadAccounts()
	if len(got) != 2 {
		t.Fatalf("got %d accounts after reload, want 2: %+v", len(got), got)
	}
	for i := range in {
		if got[i].Cookie != in[i].Cookie || got[i].APIKey != in[i].APIKey || got[i].UserID != in[i].UserID {
			t.Errorf("account %d round-trip mismatch:\n got %+v\nwant %+v", i, got[i], in[i])
		}
	}
}
