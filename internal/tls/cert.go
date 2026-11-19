package tls

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const (
	CertFileName   = "wildcard.crt"
	KeyFileName    = "wildcard.key"
	TraefikTLSConf = "tls.yml"
	DefaultTLD     = "test"
)

// CertManager handles certificate generation and Traefik TLS configuration.
type CertManager struct {
	certDir string
}

// NewCertManager creates a CertManager that stores certs in the specified directory.
// If certDir is empty, it defaults to ~/.sld/certs.
func NewCertManager(certDir string) (*CertManager, error) {
	if certDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get user home directory: %w", err)
		}
		certDir = filepath.Join(home, ".sld", "certs")
	}

	if err := os.MkdirAll(certDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cert directory: %w", err)
	}

	return &CertManager{certDir: certDir}, nil
}

// CertDir returns the absolute path to the certificate directory.
func (cm *CertManager) CertDir() string {
	return cm.certDir
}

// CertPath returns the path to the wildcard cert.
func (cm *CertManager) CertPath() string {
	return filepath.Join(cm.certDir, CertFileName)
}

// KeyPath returns the path to the wildcard key.
func (cm *CertManager) KeyPath() string {
	return filepath.Join(cm.certDir, KeyFileName)
}

// TraefikConfigPath returns the path to the generated Traefik dynamic TLS YAML.
func (cm *CertManager) TraefikConfigPath() string {
	return filepath.Join(cm.certDir, TraefikTLSConf)
}

// EnsureWildcardCert ensures a valid wildcard certificate and key exist for *.<tld>.
// It first attempts to use `mkcert` if installed.
// If mkcert is not available, it generates a self-signed wildcard certificate via Go's crypto/x509.
func (cm *CertManager) EnsureWildcardCert(tld string) error {
	if tld == "" {
		tld = DefaultTLD
	}

	certPath := cm.CertPath()
	keyPath := cm.KeyPath()

	// Check if existing cert and key are present and valid
	if cm.isValidCert(certPath) && fileExists(keyPath) {
		return cm.writeTraefikConfig()
	}

	// 1. Try mkcert first
	if isMkcertAvailable() {
		fmt.Printf("[SLD TLS] Generating local wildcard certificate using mkcert (*.%s)...\n", tld)
		if err := cm.generateWithMkcert(tld); err == nil {
			return cm.writeTraefikConfig()
		}
		fmt.Println("[SLD TLS] mkcert failed, falling back to built-in self-signed certificate...")
	}

	// 2. Fallback: Built-in certificate generation
	fmt.Printf("[SLD TLS] Generating self-signed wildcard certificate for *.%s...\n", tld)
	if err := cm.generateSelfSigned(tld); err != nil {
		return fmt.Errorf("failed to generate wildcard certificate: %w", err)
	}

	return cm.writeTraefikConfig()
}

// generateWithMkcert executes `mkcert` to create the certificate.
func (cm *CertManager) generateWithMkcert(tld string) error {
	// First ensure root CA is installed
	installCmd := exec.Command("mkcert", "-install")
	_ = installCmd.Run()

	wildcard := fmt.Sprintf("*.%s", tld)
	args := []string{
		"-cert-file", cm.CertPath(),
		"-key-file", cm.KeyPath(),
		wildcard,
		tld,
		"localhost",
		"127.0.0.1",
		"::1",
	}

	cmd := exec.Command("mkcert", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mkcert error: %s (%w)", string(out), err)
	}
	return nil
}

// generateSelfSigned creates a self-signed root CA and wildcard certificate using pure Go.
func (cm *CertManager) generateSelfSigned(tld string) error {
	wildcard := fmt.Sprintf("*.%s", tld)

	// Generate RSA private key
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("failed to generate private key: %w", err)
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return fmt.Errorf("failed to generate serial number: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Supreme Local Dev (SLD)"},
			CommonName:   wildcard,
			Country:      []string{"US"},
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour), // 1 year
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{wildcard, tld, "localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}

	// Self-sign the certificate
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privKey.PublicKey, privKey)
	if err != nil {
		return fmt.Errorf("failed to create certificate: %w", err)
	}

	// Save certificate PEM
	certOut, err := os.OpenFile(cm.CertPath(), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open cert file: %w", err)
	}
	defer certOut.Close()

	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		return fmt.Errorf("failed to write cert PEM: %w", err)
	}

	// Save private key PEM
	keyOut, err := os.OpenFile(cm.KeyPath(), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to open key file: %w", err)
	}
	defer keyOut.Close()

	privKeyBytes := x509.MarshalPKCS1PrivateKey(privKey)
	if err := pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: privKeyBytes}); err != nil {
		return fmt.Errorf("failed to write key PEM: %w", err)
	}

	return nil
}

// writeTraefikConfig writes the Traefik dynamic TLS configuration file.
// The paths inside the configuration refer to container-side paths: /etc/traefik/dynamic/certs/...
func (cm *CertManager) writeTraefikConfig() error {
	configContent := fmt.Sprintf(`tls:
  certificates:
    - certFile: /etc/traefik/certs/%s
      keyFile: /etc/traefik/certs/%s
  stores:
    default:
      defaultCertificate:
        certFile: /etc/traefik/certs/%s
        keyFile: /etc/traefik/certs/%s
`, CertFileName, KeyFileName, CertFileName, KeyFileName)

	confPath := cm.TraefikConfigPath()
	return os.WriteFile(confPath, []byte(configContent), 0644)
}

// isValidCert checks if the certificate file exists and has not expired.
func (cm *CertManager) isValidCert(certPath string) bool {
	data, err := os.ReadFile(certPath)
	if err != nil {
		return false
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return false
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false
	}

	// Ensure cert is still valid for at least 7 days
	return time.Now().Add(7 * 24 * time.Hour).Before(cert.NotAfter)
}

func isMkcertAvailable() bool {
	_, err := exec.LookPath("mkcert")
	return err == nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
