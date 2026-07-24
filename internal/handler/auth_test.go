package handler

import "testing"

func TestMagicLinkURL(t *testing.T) {
	tests := []struct {
		name   string
		domain string
		port   string
		want   string
	}{
		{
			name:   "real domain uses https and ignores port",
			domain: "patchwork.example.com",
			port:   "8080",
			want:   "https://patchwork.example.com/api/v1/auth/verify/tok123",
		},
		{
			name:   "no domain falls back to localhost with configured port",
			domain: "",
			port:   "3000",
			want:   "http://localhost:3000/api/v1/auth/verify/tok123",
		},
		{
			name:   "no domain and no port defaults to 8080",
			domain: "",
			port:   "",
			want:   "http://localhost:8080/api/v1/auth/verify/tok123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := magicLinkURL(tt.domain, tt.port, "tok123")
			if got != tt.want {
				t.Errorf("magicLinkURL(%q, %q) = %q, want %q", tt.domain, tt.port, got, tt.want)
			}
		})
	}
}
