package version

import "os"

// Version is the OpenClaw version. Can be overridden via OPENCLAW_BUNDLED_VERSION.
var Version = func() string {
	if v := os.Getenv("OPENCLAW_BUNDLED_VERSION"); v != "" {
		return v
	}
	return "0.0.1-dev"
}()
