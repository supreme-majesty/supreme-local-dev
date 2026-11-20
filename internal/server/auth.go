// Package server is SLD's control-plane HTTP API (`sld serve`): project
// management on the V2 engine, behind token authentication.
package server

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	sessionCookie  = "sld_session"
	sessionTTL     = 7 * 24 * time.Hour
	csrfHeader     = "X-SLD-Request" // must be sent with cookie-authenticated writes
	authFileName   = "auth.json"
	envAdminToken  = "SLD_ADMIN_TOKEN"
	envAllowedHost = "SLD_ALLOWED_HOSTS"
)

// Auth protects the API with an admin token. The token is shown once when it
// is generated; only its SHA-256 is stored. Browsers exchange it for an
// HttpOnly session cookie; scripts send it as a Bearer token.
type Auth struct {
	tokenHash    [32]byte
	allowedHosts []string // extra Host names besides localhost/*.localhost; "*" allows any

	mu       sync.Mutex
	sessions map[string]time.Time // session id -> expiry
}

type authFile struct {
	TokenSHA256 string    `json:"token_sha256"`
	CreatedAt   time.Time `json:"created_at"`
}

// LoadAuth reads the token hash from dataDir, or creates a new token (from
// SLD_ADMIN_TOKEN, or random) and prints it to out. reset discards the old one.
func LoadAuth(dataDir string, reset bool, out io.Writer) (*Auth, error) {
	a := &Auth{sessions: map[string]time.Time{}}
	for _, h := range strings.Split(os.Getenv(envAllowedHost), ",") {
		if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
			a.allowedHosts = append(a.allowedHosts, h)
		}
	}

	if token := os.Getenv(envAdminToken); token != "" {
		a.tokenHash = sha256.Sum256([]byte(token))
		fmt.Fprintln(out, "[SLD] Admin token taken from SLD_ADMIN_TOKEN.")
		return a, nil
	}

	path := filepath.Join(dataDir, authFileName)
	if !reset {
		if data, err := os.ReadFile(path); err == nil {
			var f authFile
			if err := json.Unmarshal(data, &f); err == nil {
				if raw, err := hex.DecodeString(f.TokenSHA256); err == nil && len(raw) == 32 {
					copy(a.tokenHash[:], raw)
					fmt.Fprintln(out, "[SLD] Using the admin token created on first start (run 'sld serve --reset-token' to issue a new one).")
					return a, nil
				}
			}
			return nil, fmt.Errorf("%s is corrupt; start with --reset-token", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}

	token, err := randomString(24)
	if err != nil {
		return nil, err
	}
	a.tokenHash = sha256.Sum256([]byte(token))
	data, _ := json.MarshalIndent(authFile{TokenSHA256: hex.EncodeToString(a.tokenHash[:]), CreatedAt: time.Now().UTC()}, "", "  ")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return nil, fmt.Errorf("failed to store admin token: %w", err)
	}
	fmt.Fprintf(out, "\n"+
		"══════════════════════════════════════════════════════════════\n"+
		"  SLD admin token (shown only once — keep it somewhere safe):\n\n"+
		"      %s\n\n"+
		"  Paste it on the dashboard's sign-in page.\n"+
		"══════════════════════════════════════════════════════════════\n\n", token)
	return a, nil
}

func randomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// checkToken compares a presented token with the stored hash in constant time.
func (a *Auth) checkToken(token string) bool {
	if token == "" {
		return false
	}
	h := sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare(h[:], a.tokenHash[:]) == 1
}

func (a *Auth) newSession() (string, error) {
	id, err := randomString(32)
	if err != nil {
		return "", err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	for sid, exp := range a.sessions { // drop expired sessions
		if now.After(exp) {
			delete(a.sessions, sid)
		}
	}
	a.sessions[id] = now.Add(sessionTTL)
	return id, nil
}

func (a *Auth) validSession(id string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	exp, ok := a.sessions[id]
	if !ok || time.Now().After(exp) {
		delete(a.sessions, id)
		return false
	}
	return true
}

func (a *Auth) endSession(id string) {
	a.mu.Lock()
	delete(a.sessions, id)
	a.mu.Unlock()
}

// hostAllowed rejects requests for unexpected Host names (DNS rebinding).
func (a *Auth) hostAllowed(hostport string) bool {
	host := strings.ToLower(hostport)
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = strings.ToLower(h)
	}
	host = strings.Trim(host, "[]")
	switch {
	case host == "localhost" || host == "127.0.0.1" || host == "::1" || strings.HasSuffix(host, ".localhost"):
		return true
	}
	for _, allowed := range a.allowedHosts {
		if allowed == "*" || allowed == host {
			return true
		}
	}
	return false
}

// publicPath lists what works without signing in: the dashboard's static
// files (so the sign-in page loads), health checks and the auth endpoints.
func publicPath(path string) bool {
	if !strings.HasPrefix(path, "/api/") {
		return true
	}
	switch path {
	case "/api/health", "/api/v2/auth/login", "/api/v2/auth/status":
		return true
	}
	return false
}

// Middleware enforces Host checks, authentication and CSRF protection.
func (a *Auth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.hostAllowed(r.Host) {
			http.Error(w, "host not allowed (set SLD_ALLOWED_HOSTS to add it)", http.StatusForbidden)
			return
		}
		if publicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		// Scripts: Authorization: Bearer <token>.
		if bearer, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
			if a.checkToken(strings.TrimSpace(bearer)) {
				next.ServeHTTP(w, r)
				return
			}
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}

		// Browsers: session cookie, plus Origin and CSRF-header checks on writes.
		cookie, err := r.Cookie(sessionCookie)
		if err != nil || !a.validSession(cookie.Value) {
			writeError(w, http.StatusUnauthorized, "sign in required")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			if origin := r.Header.Get("Origin"); origin != "" {
				u, err := url.Parse(origin)
				if err != nil || !a.hostAllowed(u.Host) {
					writeError(w, http.StatusForbidden, "cross-origin request refused")
					return
				}
			}
			if r.Header.Get(csrfHeader) == "" {
				writeError(w, http.StatusForbidden, "missing "+csrfHeader+" header")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (a *Auth) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req); err != nil || !a.checkToken(strings.TrimSpace(req.Token)) {
		time.Sleep(500 * time.Millisecond) // slow down guessing
		writeError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	id, err := a.newSession()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: id, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteStrictMode, MaxAge: int(sessionTTL.Seconds()),
		Secure: r.TLS != nil,
	})
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": true})
}

func (a *Auth) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		a.endSession(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": false})
}

func (a *Auth) handleStatus(w http.ResponseWriter, r *http.Request) {
	ok := false
	if c, err := r.Cookie(sessionCookie); err == nil {
		ok = a.validSession(c.Value)
	}
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": ok})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
