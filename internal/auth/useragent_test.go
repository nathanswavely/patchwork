package auth

import "testing"

func TestDeviceLabel(t *testing.T) {
	tests := []struct {
		name string
		ua   string
		want string
	}{
		{
			name: "empty is unknown",
			ua:   "",
			want: "Unknown device",
		},
		{
			name: "whitespace is unknown",
			ua:   "   ",
			want: "Unknown device",
		},
		{
			name: "chrome on windows",
			ua:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36",
			want: "Chrome on Windows",
		},
		{
			name: "safari on iphone",
			ua:   "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile Safari/604.1",
			want: "Safari on iPhone",
		},
		{
			name: "safari on mac",
			ua:   "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.5 Safari/605.1.15",
			want: "Safari on macOS",
		},
		{
			name: "firefox on linux",
			ua:   "Mozilla/5.0 (X11; Linux x86_64; rv:121.0) Gecko/20100101 Firefox/121.0",
			want: "Firefox on Linux",
		},
		{
			name: "edge beats chrome token",
			ua:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36 Edg/120.0",
			want: "Edge on Windows",
		},
		{
			name: "chrome on android",
			ua:   "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Mobile Safari/537.36",
			want: "Chrome on Android",
		},
		{
			name: "unrecognized token is unknown",
			ua:   "curl/8.4.0",
			want: "Unknown device",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DeviceLabel(tt.ua); got != tt.want {
				t.Errorf("DeviceLabel(%q) = %q, want %q", tt.ua, got, tt.want)
			}
		})
	}
}
