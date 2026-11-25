package orchestrate

import "testing"

func TestServed(t *testing.T) {
	cases := []struct {
		status int
		body   string
		want   bool
	}{
		{200, "<html>", true},
		{302, "", true}, // e.g. a redirect to /login
		{500, "Whoops", true},
		{404, "<h1>Not Found</h1>", true},      // the app's own 404 page
		{404, "404 page not found\n", false},   // Traefik: no route yet
		{502, "<center>nginx</center>", false}, // nginx before PHP-FPM is up
		{503, "", false},
		{504, "", false},
	}
	for _, tc := range cases {
		if got := served(tc.status, tc.body); got != tc.want {
			t.Errorf("served(%d, %q) = %v, want %v", tc.status, tc.body, got, tc.want)
		}
	}
}
