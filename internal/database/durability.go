package database

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// CheckDurability guards against the silent-data-loss failure mode where a
// containerized instance writes its SQLite database to the container's
// ephemeral writable layer instead of a mounted volume: everything works
// until the container is recreated (any image update), then the database
// vanishes. This happened in production on 2026-07-19.
//
// When running inside a container, the resolved database path must live
// under a mount point (volume or bind mount). If it doesn't, the data is
// guaranteed to be lost on recreate, so we refuse to start rather than run
// on a time bomb. Set PATCHWORK_ALLOW_EPHEMERAL=1 to override (throwaway
// demos only). Outside containers this is a no-op, as it is on platforms
// without /proc.
func CheckDurability(dbPath string) error {
	if !inContainer() {
		return nil
	}
	if os.Getenv("PATCHWORK_ALLOW_EPHEMERAL") == "1" {
		return nil
	}

	mountinfo, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		// Can't tell — don't block startup on an unreadable /proc.
		return nil
	}

	abs, err := filepath.Abs(dbPath)
	if err != nil {
		return fmt.Errorf("resolve database path %q: %w", dbPath, err)
	}

	if !onMount(abs, string(mountinfo)) {
		return fmt.Errorf("database path %q resolves to %q, which is NOT on a mounted volume — "+
			"in a container this data lives in the ephemeral writable layer and WILL BE LOST when the container is recreated (e.g. any image update). "+
			"Mount a volume over the database directory (docker-compose.yaml mounts one at /data) or set database.path in patchwork.yaml to a path inside a mount. "+
			"To run ephemerally on purpose, set PATCHWORK_ALLOW_EPHEMERAL=1", dbPath, abs)
	}
	return nil
}

// inContainer reports whether we appear to be running inside a container
// (Docker or podman).
func inContainer() bool {
	for _, marker := range []string{"/.dockerenv", "/run/.containerenv"} {
		if _, err := os.Stat(marker); err == nil {
			return true
		}
	}
	return false
}

// onMount reports whether path sits under a mount point other than the
// container's root filesystem, given /proc/self/mountinfo content. The db
// file usually doesn't exist yet, so this is a pure path-prefix check: the
// longest mount point that is an ancestor of path must not be "/".
func onMount(path string, mountinfo string) bool {
	best := ""
	for _, line := range strings.Split(mountinfo, "\n") {
		// mountinfo fields: id parent major:minor root MOUNT_POINT options...
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		mp := unescapeMountPath(fields[4])
		if mp == "/" {
			continue
		}
		if path == mp || strings.HasPrefix(path, mp+"/") {
			if len(mp) > len(best) {
				best = mp
			}
		}
	}
	return best != ""
}

// unescapeMountPath decodes the octal escapes mountinfo uses for special
// characters in paths (space = \040, tab = \011, newline = \012, \ = \134).
func unescapeMountPath(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			if v, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(v))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
