package database

import "testing"

// Trimmed from a real distroless container: overlay root, a named volume at
// /data, and the compose-mounted config file.
const sampleMountinfo = `705 663 0:83 / / rw,relatime master:212 - overlay overlay rw,lowerdir=/var/lib/docker/overlay2/l/ABC,upperdir=/var/lib/docker/overlay2/x/diff,workdir=/var/lib/docker/overlay2/x/work
706 705 0:86 / /proc rw,nosuid,nodev,noexec,relatime - proc proc rw
710 705 0:87 / /dev rw,nosuid - tmpfs tmpfs rw,size=65536k,mode=755
712 705 8:1 /var/lib/docker/volumes/patchwork_data/_data /data rw,relatime - ext4 /dev/sda1 rw
713 705 8:1 /opt/patchwork/patchwork.yaml /patchwork.yaml ro,relatime - ext4 /dev/sda1 rw
714 705 8:1 /var/tmp/with\040space /odd\040mount rw,relatime - ext4 /dev/sda1 rw`

func TestOnMount(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		// The production incident: relative "data/patchwork.db" resolved
		// under the workdir in the ephemeral overlay layer.
		{"/home/nonroot/data/patchwork.db", false},
		// The fixed layout: workdir /data is the volume.
		{"/data/patchwork.db", true},
		{"/data/data/patchwork.db", true},
		{"/data", true},
		// Prefix must be component-wise, not string-wise.
		{"/database/patchwork.db", false},
		{"/patchwork.db", false},
		// Escaped space in the mount point.
		{"/odd mount/patchwork.db", true},
	}
	for _, tt := range tests {
		if got := onMount(tt.path, sampleMountinfo); got != tt.want {
			t.Errorf("onMount(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestUnescapeMountPath(t *testing.T) {
	tests := []struct{ in, want string }{
		{"/data", "/data"},
		{`/odd\040mount`, "/odd mount"},
		{`/tab\011here`, "/tab\there"},
		{`/trailing\`, `/trailing\`},
		{`/bad\zzz`, `/bad\zzz`},
	}
	for _, tt := range tests {
		if got := unescapeMountPath(tt.in); got != tt.want {
			t.Errorf("unescapeMountPath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
