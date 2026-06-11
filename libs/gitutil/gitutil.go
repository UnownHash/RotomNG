// Package gitutil provides utilities for extracting git build information.
package gitutil

import "runtime/debug"

// GetGitBuildSHA returns the git revision from build info, or "unknown".
func GetGitBuildSHA() string {
	info, ok := debug.ReadBuildInfo()
	if ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				return setting.Value
			}
		}
	}
	return "unknown"
}
