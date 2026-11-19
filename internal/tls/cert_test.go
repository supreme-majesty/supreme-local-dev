package tls

import (
	"crypto/x509"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	m, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return m
}

func readCert(t *testing.T, path string) *x509.Certificate {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	cert, err := parseCertPEM(data)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return cert
}

func TestEnsureCA_CreatesOnceAndIsReused(t *testing.T) {
	m := newTestManager(t)

	created, err := m.EnsureCA()
	if err != nil || !created {
		t.Fatalf("first EnsureCA: created=%v err=%v", created, err)
	}
	first := readCert(t, m.CACertPath())
	if !first.IsCA {
		t.Fatal("CA certificate is not marked as a CA")
	}

	created, err = m.EnsureCA()
	if err != nil || created {
		t.Fatalf("second EnsureCA: created=%v err=%v", created, err)
	}
	if !readCert(t, m.CACertPath()).Equal(first) {
		t.Fatal("CA was regenerated on second call")
	}

	info, err := os.Stat(m.caKeyPath())
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode&0o077 != 0 && os.PathSeparator == '/' {
		t.Errorf("CA key permissions too open: %v", mode)
	}
}

func TestEnsureCert_ChainsToCAAndCoversHosts(t *testing.T) {
	m := newTestManager(t)
	if _, err := m.EnsureCA(); err != nil {
		t.Fatal(err)
	}

	issued, err := m.EnsureCert("shop-1234abcd", []string{"Shop.localhost", "api.shop.localhost", "shop.localhost"})
	if err != nil || !issued {
		t.Fatalf("EnsureCert: issued=%v err=%v", issued, err)
	}

	leaf := readCert(t, m.CertPath("shop-1234abcd"))
	pool := x509.NewCertPool()
	pool.AddCert(readCert(t, m.CACertPath()))
	for _, host := range []string{"shop.localhost", "api.shop.localhost"} {
		if _, err := leaf.Verify(x509.VerifyOptions{DNSName: host, Roots: pool}); err != nil {
			t.Errorf("leaf does not verify for %s: %v", host, err)
		}
	}
	if _, err := leaf.Verify(x509.VerifyOptions{DNSName: "other.localhost", Roots: pool}); err == nil {
		t.Error("leaf unexpectedly verifies for an unlisted host")
	}

	hosts, err := m.CertHosts("shop-1234abcd")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"api.shop.localhost", "shop.localhost"}; !slices.Equal(hosts, want) {
		t.Errorf("hosts = %v, want %v (deduplicated, lower-cased)", hosts, want)
	}

	conf, err := os.ReadFile(filepath.Join(m.TraefikDir(), "cert-shop-1234abcd.yml"))
	if err != nil {
		t.Fatalf("traefik config not written: %v", err)
	}
	for _, want := range []string{"/etc/traefik/certs/shop-1234abcd.crt", "/etc/traefik/certs/shop-1234abcd.key"} {
		if !strings.Contains(string(conf), want) {
			t.Errorf("traefik config missing %q:\n%s", want, conf)
		}
	}
}

func TestEnsureCert_ReissuesOnlyWhenNeeded(t *testing.T) {
	m := newTestManager(t)
	if _, err := m.EnsureCA(); err != nil {
		t.Fatal(err)
	}
	if _, err := m.EnsureCert("app", []string{"app.localhost"}); err != nil {
		t.Fatal(err)
	}
	first := readCert(t, m.CertPath("app"))

	issued, err := m.EnsureCert("app", []string{"APP.localhost"})
	if err != nil || issued {
		t.Fatalf("same hosts: issued=%v err=%v (want reuse)", issued, err)
	}

	issued, err = m.EnsureCert("app", []string{"app.localhost", "www.app.localhost"})
	if err != nil || !issued {
		t.Fatalf("changed hosts: issued=%v err=%v (want reissue)", issued, err)
	}
	if readCert(t, m.CertPath("app")).Equal(first) {
		t.Fatal("certificate was not replaced after host change")
	}
}

func TestEnsureCert_ReissuesAfterCAChange(t *testing.T) {
	m := newTestManager(t)
	if _, err := m.EnsureCA(); err != nil {
		t.Fatal(err)
	}
	if _, err := m.EnsureCert("app", []string{"app.localhost"}); err != nil {
		t.Fatal(err)
	}

	// Replace the CA with a different one.
	other := newTestManager(t)
	if _, err := other.EnsureCA(); err != nil {
		t.Fatal(err)
	}
	certPEM, _ := os.ReadFile(other.CACertPath())
	keyPEM, _ := os.ReadFile(other.caKeyPath())
	if err := m.ImportCA(certPEM, keyPEM); err != nil {
		t.Fatalf("ImportCA: %v", err)
	}

	issued, err := m.EnsureCert("app", []string{"app.localhost"})
	if err != nil || !issued {
		t.Fatalf("after CA change: issued=%v err=%v (want reissue)", issued, err)
	}
	if err := readCert(t, m.CertPath("app")).CheckSignatureFrom(readCert(t, m.CACertPath())); err != nil {
		t.Fatalf("reissued cert not signed by new CA: %v", err)
	}
}

func TestEnsureDefault_WritesDefaultStore(t *testing.T) {
	m := newTestManager(t)
	if err := m.EnsureDefault(); err != nil {
		t.Fatalf("EnsureDefault: %v", err)
	}
	leaf := readCert(t, m.CertPath(DefaultCert))
	if !slices.Contains(leaf.DNSNames, "localhost") || len(leaf.IPAddresses) != 2 {
		t.Errorf("default cert SANs: dns=%v ip=%v", leaf.DNSNames, leaf.IPAddresses)
	}
	conf, err := os.ReadFile(filepath.Join(m.TraefikDir(), "tls.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(conf), "defaultCertificate") || !strings.Contains(string(conf), "/etc/traefik/certs/default.crt") {
		t.Errorf("unexpected default store config:\n%s", conf)
	}
	// The CA key must never be in a directory mounted into the router.
	entries, _ := os.ReadDir(m.CertDir())
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "ca.") {
			t.Errorf("CA material found in router-mounted cert dir: %s", e.Name())
		}
	}
}

func TestRemoveCert(t *testing.T) {
	m := newTestManager(t)
	if err := m.EnsureDefault(); err != nil {
		t.Fatal(err)
	}
	if _, err := m.EnsureCert("app", []string{"app.localhost"}); err != nil {
		t.Fatal(err)
	}
	if err := m.RemoveCert("app"); err != nil {
		t.Fatalf("RemoveCert: %v", err)
	}
	for _, p := range []string{m.CertPath("app"), m.KeyPath("app"), filepath.Join(m.TraefikDir(), "cert-app.yml")} {
		if fileExists(p) {
			t.Errorf("%s still exists", p)
		}
	}
	if err := m.RemoveCert("app"); err != nil {
		t.Errorf("RemoveCert on missing cert should be a no-op: %v", err)
	}
	if err := m.RemoveCert(DefaultCert); err == nil {
		t.Error("RemoveCert must refuse to delete the default certificate")
	}
}

func TestEnsureCert_RejectsBadInput(t *testing.T) {
	m := newTestManager(t)
	if _, err := m.EnsureCA(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"", "../escape", "UPPER", "a/b"} {
		if _, err := m.EnsureCert(name, []string{"x.localhost"}); err == nil {
			t.Errorf("EnsureCert(%q) should fail", name)
		}
	}
	if _, err := m.EnsureCert("empty", nil); err == nil {
		t.Error("EnsureCert with no hosts should fail")
	}
}

func TestImportCA_RejectsMismatchedKey(t *testing.T) {
	a, b := newTestManager(t), newTestManager(t)
	if _, err := a.EnsureCA(); err != nil {
		t.Fatal(err)
	}
	if _, err := b.EnsureCA(); err != nil {
		t.Fatal(err)
	}
	certPEM, _ := os.ReadFile(a.CACertPath())
	wrongKey, _ := os.ReadFile(b.caKeyPath())
	if err := newTestManager(t).ImportCA(certPEM, wrongKey); err == nil {
		t.Fatal("ImportCA accepted a key that does not match the certificate")
	}
}
