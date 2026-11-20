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
	allowedHosts []string // extra Host names besides localhost/*.localhost; "*" allows any

	mu        sync.Mutex
	tokenHash [32]byte
	sessions  map[string]time.Time // session id -> expiry

	// path is the stored hash. When `sld token` replaces it, the running
	// server notices the new modification time and accepts the new token.
	path    string
	modTime time.Time
}

type authFile struct {
	TokenSHA256 string    `json:"token_sha256"`
	CreatedAt   time.Time `json:"created_at"`
}

// ErrTokenFromEnv is returned by IssueToken when SLD_ADMIN_TOKEN fixes the token.
var ErrTokenFromEnv = errors.New("the admin token is set by SLD_ADMIN_TOKEN; change that variable instead")

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

	a.path = filepath.Join(dataDir, authFileName)
	if !reset {
		err := a.reload()
		if err == nil {
			fmt.Fprintln(out, "[SLD] Using the stored admin token. Lost it? Run 'sld token' in this container for a new one.")
			return a, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}

	token, err := IssueToken(dataDir)
	if err != nil {
		return nil, err
	}
	if err := a.reload(); err != nil {
		return nil, err
	}
	fmt.Fprintf(out, "\n"+
		"══════════════════════════════════════════════════════════════\n"+
		"  SLD admin token (shown only once):\n\n"+
		"      %s\n\n"+
		"  Paste it on the dashboard's sign-in page. Lost it? Run\n"+
		"  'sld token' in this container (make token) for a new one.\n"+
		"══════════════════════════════════════════════════════════════\n\n", token)
	return a, nil
}

// IssueToken creates a new random admin token, stores its hash in dataDir
// (replacing the old one) and returns the token. A running server picks the
// new hash up on its next request; existing dashboard sessions stay signed in.
func IssueToken(dataDir string) (string, error) {
	if os.Getenv(envAdminToken) != "" {
		return "", ErrTokenFromEnv
	}
	token, err := randomString(24)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256([]byte(token))
	data, _ := json.MarshalIndent(authFile{TokenSHA256: hex.EncodeToString(hash[:]), CreatedAt: time.Now().UTC()}, "", "  ")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dataDir, authFileName)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return "", fmt.Errorf("failed to store admin token: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", fmt.Errorf("failed to store admin token: %w", err)
	}
	return token, nil
}

// reload reads the stored hash if the file changed since it was last read.
func (a *Auth) reload() error {
	info, err := os.Stat(a.path)
	if err != nil {
		return err
	}
	a.mu.Lock()
	unchanged := info.ModTime().Equal(a.modTime)
	a.mu.Unlock()
	if unchanged {
		return nil
	}
	data, err := os.ReadFile(a.path)
	if err != nil {
		return err
	}
	var f authFile
	if err := json.Unmarshal(data, &f); err != nil {
		return fmt.Errorf("%s is corrupt; run 'sld token' for a new token", a.path)
	}
	raw, err := hex.DecodeString(f.TokenSHA256)
	if err != nil || len(raw) != 32 {
		return fmt.Errorf("%s is corrupt; run 'sld token' for a new token", a.path)
	}
	a.mu.Lock()
	copy(a.tokenHash[:], raw)
	a.modTime = info.ModTime()
	a.mu.Unlock()
	return nil
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
	if a.path != "" {
		_ = a.reload() // keep the last good hash if the file is briefly unreadable
	}
	h := sha256.Sum256([]byte(token))
	a.mu.Lock()
	defer a.mu.Unlock()
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
