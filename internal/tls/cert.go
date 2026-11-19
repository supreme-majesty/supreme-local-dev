// Package tls manages SLD's local certificate authority and the certificates
// served by the Traefik router.
//
// Layout under the SLD data directory:
//
//	ca/ca.crt, ca/ca.key           local CA (never mounted into other containers)
//	certs/<name>.crt, <name>.key   leaf certificates -> router /etc/traefik/certs
//	traefik/tls.yml                default certificate store -> router /etc/traefik/dynamic
//	traefik/cert-<name>.yml        one Traefik dynamic file per leaf certificate
//
// Leaf certificates list explicit host names. Browsers reject second-level
// wildcards such as *.localhost or *.test, so each project gets its own cert.
package tls

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

const (
	caDirName      = "ca"
	certDirName    = "certs"
	traefikDirName = "traefik"

	caCertFile     = "ca.crt"
	caKeyFile      = "ca.key"
	defaultStore   = "tls.yml"
	DefaultCert    = "default"
	routerCertsDir = "/etc/traefik/certs"

	caValidity   = 10 * 365 * 24 * time.Hour
	leafValidity = 397 * 24 * time.Hour // stay within browser limits for leaf lifetimes
	renewBefore  = 30 * 24 * time.Hour
)

// DefaultHosts are the names on the router's default certificate.
var DefaultHosts = []string{"localhost", "sld.localhost", "127.0.0.1", "::1"}

var certNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,99}$`)

// Manager owns the CA and leaf certificates under one data directory.
type Manager struct {
	dataDir string
}

// NewManager creates the directory layout under dataDir.
func NewManager(dataDir string) (*Manager, error) {
	if dataDir == "" {
		return nil, errors.New("tls: data directory is required")
	}
	m := &Manager{dataDir: dataDir}
	for dir, mode := range map[string]os.FileMode{
		m.CADir():      0o700,
		m.CertDir():    0o755,
		m.TraefikDir(): 0o755,
	} {
		if err := os.MkdirAll(dir, mode); err != nil {
			return nil, fmt.Errorf("tls: failed to create %s: %w", dir, err)
		}
	}
	return m, nil
}

// CADir holds the CA certificate and key.
func (m *Manager) CADir() string { return filepath.Join(m.dataDir, caDirName) }

// CertDir holds leaf certificates; mount it at /etc/traefik/certs in the router.
func (m *Manager) CertDir() string { return filepath.Join(m.dataDir, certDirName) }

// TraefikDir holds Traefik dynamic TLS config; mount it at /etc/traefik/dynamic.
func (m *Manager) TraefikDir() string { return filepath.Join(m.dataDir, traefikDirName) }

// CACertPath is the CA certificate users import into their trust store.
func (m *Manager) CACertPath() string { return filepath.Join(m.CADir(), caCertFile) }

func (m *Manager) caKeyPath() string { return filepath.Join(m.CADir(), caKeyFile) }

// CertPath returns the certificate path for a leaf name.
func (m *Manager) CertPath(name string) string { return filepath.Join(m.CertDir(), name+".crt") }

// KeyPath returns the key path for a leaf name.
func (m *Manager) KeyPath(name string) string { return filepath.Join(m.CertDir(), name+".key") }

func (m *Manager) traefikCertConfigPath(name string) string {
	return filepath.Join(m.TraefikDir(), "cert-"+name+".yml")
}

// HasCA reports whether a CA certificate and key exist.
func (m *Manager) HasCA() bool {
	return fileExists(m.CACertPath()) && fileExists(m.caKeyPath())
}

// EnsureCA loads the CA, creating a new one if none exists. It reports whether
// a CA was created (users then need to trust it once).
func (m *Manager) EnsureCA() (bool, error) {
	if _, _, err := m.loadCA(); err == nil {
		return false, nil
	} else if m.HasCA() {
		return false, fmt.Errorf("tls: existing CA is unreadable: %w", err)
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return false, fmt.Errorf("tls: failed to generate CA key: %w", err)
	}
	serial, err := randomSerial()
	if err != nil {
		return false, err
	}
	suffix := hex.EncodeToString(serial.Bytes())
	if len(suffix) > 8 {
		suffix = suffix[:8]
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"Supreme Local Dev (SLD)"},
			CommonName:   "SLD Development CA " + suffix,
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(caValidity),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return false, fmt.Errorf("tls: failed to create CA certificate: %w", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return false, fmt.Errorf("tls: failed to encode CA key: %w", err)
	}
	if err := m.writeCA(pemBytes("CERTIFICATE", der), pemBytes("PRIVATE KEY", keyDER)); err != nil {
		return false, err
	}
	return true, nil
}

// ImportCA installs an existing CA (e.g. mkcert's root, which is often already
// trusted by the user's browsers) in place of generating one.
func (m *Manager) ImportCA(certPEM, keyPEM []byte) error {
	cert, err := parseCertPEM(certPEM)
	if err != nil {
		return fmt.Errorf("tls: invalid CA certificate: %w", err)
	}
	if !cert.IsCA {
		return errors.New("tls: imported certificate is not a CA")
	}
	key, err := parseKeyPEM(keyPEM)
	if err != nil {
		return fmt.Errorf("tls: invalid CA key: %w", err)
	}
	if !publicKeysEqual(cert.PublicKey, key.Public()) {
		return errors.New("tls: CA key does not match certificate")
	}
	return m.writeCA(certPEM, keyPEM)
}

// ImportMkcertCA imports mkcert's root CA when no SLD CA exists yet and mkcert
// is installed on this machine. Only meaningful when SLD runs on the host.
func (m *Manager) ImportMkcertCA() (bool, error) {
	if m.HasCA() {
		return false, nil
	}
	if _, err := exec.LookPath("mkcert"); err != nil {
		return false, nil
	}
	out, err := exec.Command("mkcert", "-CAROOT").Output()
	if err != nil {
		return false, nil
	}
	root := strings.TrimSpace(string(out))
	certPEM, err1 := os.ReadFile(filepath.Join(root, "rootCA.pem"))
	keyPEM, err2 := os.ReadFile(filepath.Join(root, "rootCA-key.pem"))
	if err1 != nil || err2 != nil {
		return false, nil
	}
	if err := m.ImportCA(certPEM, keyPEM); err != nil {
		return false, err
	}
	return true, nil
}

// EnsureDefault makes sure the CA, the router's default certificate and the
// default-store Traefik config exist.
func (m *Manager) EnsureDefault() error {
	if _, err := m.EnsureCA(); err != nil {
		return err
	}
	if _, err := m.EnsureCert(DefaultCert, DefaultHosts); err != nil {
		return err
	}
	content := fmt.Sprintf(`# Managed by SLD. Do not edit.
tls:
  stores:
    default:
      defaultCertificate:
        certFile: %[1]s/%[2]s.crt
        keyFile: %[1]s/%[2]s.key
`, routerCertsDir, DefaultCert)
	return writeFileIfChanged(filepath.Join(m.TraefikDir(), defaultStore), []byte(content), 0o644)
}

// EnsureCert issues (or re-issues) the leaf certificate called name for hosts
// and writes its Traefik dynamic config. It reports whether a new certificate
// was issued. An existing certificate is kept when it is signed by the current
// CA, covers exactly the same hosts and is not close to expiry.
func (m *Manager) EnsureCert(name string, hosts []string) (bool, error) {
	if !certNamePattern.MatchString(name) {
		return false, fmt.Errorf("tls: invalid certificate name %q", name)
	}
	hosts = normalizeHosts(hosts)
	if len(hosts) == 0 {
		return false, fmt.Errorf("tls: certificate %q needs at least one host name", name)
	}

	caCert, caKey, err := m.loadCA()
	if err != nil {
		return false, fmt.Errorf("tls: CA not available (run init first): %w", err)
	}

	issued := false
	if !m.leafIsCurrent(name, hosts, caCert) {
		if err := m.issueLeaf(name, hosts, caCert, caKey); err != nil {
			return false, err
		}
		issued = true
	}

	content := fmt.Sprintf(`# Managed by SLD. Do not edit.
tls:
  certificates:
    - certFile: %[1]s/%[2]s.crt
      keyFile: %[1]s/%[2]s.key
`, routerCertsDir, name)
	if err := writeFileIfChanged(m.traefikCertConfigPath(name), []byte(content), 0o644); err != nil {
		return issued, err
	}
	return issued, nil
}

// RemoveCert deletes a leaf certificate and its Traefik config. Missing files are ignored.
func (m *Manager) RemoveCert(name string) error {
	if !certNamePattern.MatchString(name) || name == DefaultCert {
		return fmt.Errorf("tls: refusing to remove certificate %q", name)
	}
	// Remove the Traefik reference first so the router never points at a missing file.
	for _, p := range []string{m.traefikCertConfigPath(name), m.CertPath(name), m.KeyPath(name)} {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("tls: failed to remove %s: %w", p, err)
		}
	}
	return nil
}

// CertHosts returns the host names on an issued leaf certificate.
func (m *Manager) CertHosts(name string) ([]string, error) {
	data, err := os.ReadFile(m.CertPath(name))
	if err != nil {
		return nil, err
	}
	cert, err := parseCertPEM(data)
	if err != nil {
		return nil, err
	}
	return certHosts(cert), nil
}

func (m *Manager) leafIsCurrent(name string, hosts []string, caCert *x509.Certificate) bool {
	data, err := os.ReadFile(m.CertPath(name))
	if err != nil || !fileExists(m.KeyPath(name)) {
		return false
	}
	cert, err := parseCertPEM(data)
	if err != nil {
		return false
	}
	if time.Now().Add(renewBefore).After(cert.NotAfter) {
		return false
	}
	if cert.CheckSignatureFrom(caCert) != nil {
		return false
	}
	return slices.Equal(certHosts(cert), hosts)
}

func (m *Manager) issueLeaf(name string, hosts []string, caCert *x509.Certificate, caKey crypto.Signer) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("tls: failed to generate key: %w", err)
	}
	serial, err := randomSerial()
	if err != nil {
		return err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"Supreme Local Dev (SLD)"},
			CommonName:   hosts[0],
		},
		NotBefore:   time.Now().Add(-time.Hour),
		NotAfter:    time.Now().Add(leafValidity),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, h)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &key.PublicKey, caKey)
	if err != nil {
		return fmt.Errorf("tls: failed to sign certificate %q: %w", name, err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return fmt.Errorf("tls: failed to encode key: %w", err)
	}
	// Write the key before the cert: a cert without its key is never "current".
	if err := writeFileAtomic(m.KeyPath(name), pemBytes("PRIVATE KEY", keyDER), 0o600); err != nil {
		return err
	}
	return writeFileAtomic(m.CertPath(name), pemBytes("CERTIFICATE", der), 0o644)
}

func (m *Manager) loadCA() (*x509.Certificate, crypto.Signer, error) {
	certPEM, err := os.ReadFile(m.CACertPath())
	if err != nil {
		return nil, nil, err
	}
	keyPEM, err := os.ReadFile(m.caKeyPath())
	if err != nil {
		return nil, nil, err
	}
	cert, err := parseCertPEM(certPEM)
	if err != nil {
		return nil, nil, err
	}
	key, err := parseKeyPEM(keyPEM)
	if err != nil {
		return nil, nil, err
	}
	return cert, key, nil
}

func (m *Manager) writeCA(certPEM, keyPEM []byte) error {
	if err := writeFileAtomic(m.caKeyPath(), keyPEM, 0o600); err != nil {
		return err
	}
	return writeFileAtomic(m.CACertPath(), certPEM, 0o644)
}

// --- helpers ---

func normalizeHosts(hosts []string) []string {
	seen := make(map[string]bool, len(hosts))
	out := make([]string, 0, len(hosts))
	for _, h := range hosts {
		h = strings.ToLower(strings.TrimSpace(h))
		if h == "" || seen[h] {
			continue
		}
		seen[h] = true
		out = append(out, h)
	}
	slices.Sort(out)
	return out
}

func certHosts(cert *x509.Certificate) []string {
	hosts := append([]string(nil), cert.DNSNames...)
	for _, ip := range cert.IPAddresses {
		hosts = append(hosts, ip.String())
	}
	return normalizeHosts(hosts)
}

func parseCertPEM(data []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("no CERTIFICATE PEM block")
	}
	return x509.ParseCertificate(block.Bytes)
}

func parseKeyPEM(data []byte) (crypto.Signer, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("no PEM block")
	}
	var (
		key any
		err error
	)
	switch block.Type {
	case "PRIVATE KEY":
		key, err = x509.ParsePKCS8PrivateKey(block.Bytes)
	case "RSA PRIVATE KEY":
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	case "EC PRIVATE KEY":
		key, err = x509.ParseECPrivateKey(block.Bytes)
	default:
		return nil, fmt.Errorf("unsupported key type %q", block.Type)
	}
	if err != nil {
		return nil, err
	}
	signer, ok := key.(crypto.Signer)
	if !ok {
		return nil, errors.New("key cannot sign")
	}
	return signer, nil
}

func publicKeysEqual(a, b crypto.PublicKey) bool {
	type equaler interface{ Equal(crypto.PublicKey) bool }
	if e, ok := a.(equaler); ok {
		return e.Equal(b)
	}
	return false
}

func randomSerial() (*big.Int, error) {
	n, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("tls: failed to generate serial: %w", err)
	}
	return n, nil
}

func pemBytes(typ string, der []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der})
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("tls: failed to write %s: %w", path, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("tls: failed to write %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("tls: failed to write %s: %w", path, err)
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return fmt.Errorf("tls: failed to chmod %s: %w", path, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("tls: failed to write %s: %w", path, err)
	}
	return nil
}

// writeFileIfChanged avoids rewriting Traefik config that has not changed, so
// the router's file watcher does not reload needlessly.
func writeFileIfChanged(path string, data []byte, mode os.FileMode) error {
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, data) {
		return nil
	}
	return writeFileAtomic(path, data, mode)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
