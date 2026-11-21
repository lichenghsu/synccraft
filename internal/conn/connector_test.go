package conn

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeTestCerts writes a self-signed CA and a client certificate signed by it.
func writeTestCerts(t *testing.T) (caPath, certPath, keyPath string) {
	t.Helper()
	dir := t.TempDir()

	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "synccraft test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}

	clientKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	clientTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "app"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	clientDER, err := x509.CreateCertificate(rand.Reader, clientTmpl, caTmpl, &clientKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, _ := x509.MarshalECPrivateKey(clientKey)

	write := func(name, typ string, der []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der}), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	return write("ca.pem", "CERTIFICATE", caDER), write("client.pem", "CERTIFICATE", clientDER), write("client.key", "EC PRIVATE KEY", keyDER)
}

func TestLoadTLSConfig(t *testing.T) {
	ca, cert, key := writeTestCerts(t)

	tc, err := loadTLSConfig(&TLSFiles{CACertPath: ca, ClientCertPath: cert, ClientKeyPath: key}, "db.internal")
	if err != nil {
		t.Fatalf("loadTLSConfig() = %v", err)
	}
	if tc.ServerName != "db.internal" {
		t.Errorf("ServerName = %q", tc.ServerName)
	}
	if tc.RootCAs == nil || len(tc.Certificates) != 1 {
		t.Errorf("RootCAs set = %v, client certs = %d", tc.RootCAs != nil, len(tc.Certificates))
	}

	caOnly, err := loadTLSConfig(&TLSFiles{CACertPath: ca}, "h")
	if err != nil || len(caOnly.Certificates) != 0 {
		t.Errorf("CA only: %v, %d client certs", err, len(caOnly.Certificates))
	}
}

func TestLoadTLSConfigErrors(t *testing.T) {
	ca, cert, _ := writeTestCerts(t)
	garbage := filepath.Join(t.TempDir(), "garbage.pem")
	os.WriteFile(garbage, []byte("not a certificate"), 0o600)

	cases := map[string]*TLSFiles{
		"missing CA":       {CACertPath: filepath.Join(t.TempDir(), "nope.pem")},
		"garbage CA":       {CACertPath: garbage},
		"key does not fit": {CACertPath: ca, ClientCertPath: cert, ClientKeyPath: garbage},
	}
	for name, files := range cases {
		if _, err := loadTLSConfig(files, "h"); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
