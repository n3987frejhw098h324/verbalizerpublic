package main

import (
	"fmt"

	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/config"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/color"
)

var verboseEnabled = config.GetBool("verbose")

func verboseln(a ...any) {
	if !verboseEnabled {
		return
	}
	color.Verbose.Println(a...)
}

func verbosef(format string, a ...any) {
	if !verboseEnabled {
		return
	}
	color.Verbose.Println(fmt.Sprintf(format, a...))
}

func verboseConfigDump() {
	if !verboseEnabled {
		return
	}
	verboseln("Verbose logging enabled. Active configuration:")
	for _, key := range []string{
		"port",
		"print_successful_reuploads",
		"rate_limit_pause_seconds",
		"max_rate_limit_waits",
		"mesh_uploads_per_minute",
		"sound_uploads_per_minute",
		"sound_permissions_per_minute",
		"animation_uploads_per_minute",
		"animation_max_concurrent_uploads",
		"decal_uploads_per_minute",
		"assets_info_per_minute",
		"item_details_per_minute",
		"gamepass_creates_per_minute",
		"developerproduct_creates_per_minute",
		"badge_creates_per_minute",
	} {
		verbosef("  %s = %s", key, config.Get(key))
	}
}
