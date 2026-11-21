package api

import (
	"net/http/httptest"
	"testing"
)

func TestSameOrigin(t *testing.T) {
	cases := []struct {
		origin, host string
		want         bool
	}{
		{"http://localhost:2025", "localhost:2025", true},
		{"http://LOCALHOST:2025", "localhost:2025", true},
		{"http://localhost", "localhost:80", true},
		{"https://sld.localhost", "sld.localhost:443", true},
		{"http://[::1]:2025", "[::1]:2025", true},
		{"http://localhost:3000", "localhost:2025", false},
		{"http://shop.localhost:2025", "localhost:2025", false},
		{"https://localhost:2025", "localhost", false},
		{"null", "localhost:2025", false},
	}
	for _, c := range cases {
		if got := SameOrigin(c.origin, c.host); got != c.want {
			t.Errorf("SameOrigin(%q, %q) = %v, want %v", c.origin, c.host, got, c.want)
		}
	}
}

func TestWebSocketCheckOrigin(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/ws", nil)
	req.Host = "localhost:2025"
	if !upgrader.CheckOrigin(req) {
		t.Error("a request without Origin (non-browser client) should be allowed")
	}
	req.Header.Set("Origin", "http://localhost:2025")
	if !upgrader.CheckOrigin(req) {
		t.Error("the dashboard's own origin should be allowed")
	}
	req.Header.Set("Origin", "http://localhost:5173")
	if upgrader.CheckOrigin(req) {
		t.Error("another local port must be refused")
	}
}
