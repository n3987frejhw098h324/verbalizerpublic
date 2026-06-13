package config

import (
	"bufio"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/n3987frejhw098h324/verbalizerpublic/internal/files"
)

var (
	config        = make(map[string]string, 0)
	defaultConfig = map[string]string{
		"port":    "38073",
		"cookie":  "",
		"api_key": "",

		"print_successful_reuploads": "false",
		"verbose":                    "false",

		"rate_limit_pause_seconds": "5",
		"max_rate_limit_waits":     "60",

		"mesh_uploads_per_minute":          "3000",
		"sound_uploads_per_minute":         "120",
		"sound_permissions_per_minute":     "60",
		"animation_uploads_per_minute":     "420",
		"animation_max_concurrent_uploads": "24",
		"decal_uploads_per_minute":         "180",
		"assets_info_per_minute":           "100",

		"item_details_per_minute":             "100",
		"gamepass_creates_per_minute":         "60",
		"developerproduct_creates_per_minute": "60",
		"badge_creates_per_minute":            "60",
	}
)

func init() {
	contents, err := files.Read("config.ini")
	if err != nil && !os.IsNotExist(err) {
		log.Printf("failed reading config.ini, using defaults: %v", err)
	}
	if err != nil {
		contents = ""
	}

	scanner := bufio.NewScanner(strings.NewReader(contents))
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

	for i, v := range defaultConfig {
		if _, exists := config[i]; exists {
			continue
		}
		config[i] = v
	}

	migrateLegacySecretFiles()
}

func migrateLegacySecretFiles() {
	if strings.TrimSpace(config["cookie"]) == "" {
		if data, err := files.Read("cookie.txt"); err == nil {
			cookie, embeddedKey := parseLegacyCookieFile(data)
			if cookie != "" {
				config["cookie"] = cookie
			}
			if embeddedKey != "" && strings.TrimSpace(config["api_key"]) == "" {
				config["api_key"] = embeddedKey
			}
		}
	}

	if strings.TrimSpace(config["api_key"]) == "" {
		if data, err := files.Read("api-key.txt"); err == nil && strings.TrimSpace(data) != "" {
			config["api_key"] = strings.TrimSpace(data)
		}
	}
}

func parseLegacyCookieFile(content string) (cookie, apiKey string) {
	for _, line := range strings.Split(strings.TrimSpace(content), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "api-key:") {
			apiKey = strings.TrimSpace(strings.TrimPrefix(line, "api-key:"))
		} else if line != "" {
			cookie = line
		}
	}
	return cookie, apiKey
}

func Get(key string) string {
	return config[key]
}

func GetInt(key string) int {
	if n, err := strconv.Atoi(strings.TrimSpace(config[key])); err == nil && n > 0 {
		return n
	}
	if n, err := strconv.Atoi(strings.TrimSpace(defaultConfig[key])); err == nil && n > 0 {
		return n
	}
	return 0
}

func GetBool(key string) bool {
	if b, err := strconv.ParseBool(strings.TrimSpace(config[key])); err == nil {
		return b
	}
	b, _ := strconv.ParseBool(strings.TrimSpace(defaultConfig[key]))
	return b
}

func Set(key string, value string) {
	config[key] = value
}

func Save() error {
	var out strings.Builder
	keys := make([]string, 0, len(config))
	for key := range config {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		out.WriteString(key)
		out.WriteByte('=')
		out.WriteString(config[key])
		out.WriteByte('\n')
	}
	return files.Write("config.ini", out.String())
}
