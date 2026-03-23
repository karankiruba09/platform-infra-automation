package checks

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

func TestParseCertsFromB64_PEM(t *testing.T) {
	cert, pemBytes := makeTestCert(t, time.Now().Add(24*time.Hour))
	b64 := base64.StdEncoding.EncodeToString(pemBytes)

	certs, err := parseCertsFromB64(b64)
	if err != nil {
		t.Fatalf("parseCertsFromB64: %v", err)
	}
	if len(certs) != 1 {
		t.Fatalf("expected 1 cert, got %d", len(certs))
	}
	if certs[0].Subject.CommonName != cert.Subject.CommonName {
		t.Fatalf("unexpected subject CN: got %q want %q", certs[0].Subject.CommonName, cert.Subject.CommonName)
	}
	if !certs[0].NotAfter.Equal(cert.NotAfter) {
		t.Fatalf("unexpected notAfter: got %s want %s", certs[0].NotAfter, cert.NotAfter)
	}
}

func TestParseCertsFromB64_DER(t *testing.T) {
	cert, _ := makeTestCert(t, time.Now().Add(24*time.Hour))
	b64 := base64.StdEncoding.EncodeToString(cert.Raw)

	certs, err := parseCertsFromB64(b64)
	if err != nil {
		t.Fatalf("parseCertsFromB64: %v", err)
	}
	if len(certs) != 1 {
		t.Fatalf("expected 1 cert, got %d", len(certs))
	}
	if certs[0].Subject.CommonName != cert.Subject.CommonName {
		t.Fatalf("unexpected subject CN: got %q want %q", certs[0].Subject.CommonName, cert.Subject.CommonName)
	}
}

func makeTestCert(t *testing.T, notAfter time.Time) (*x509.Certificate, []byte) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		t.Fatalf("rand.Int: %v", err)
	}

	tpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: "test-ca",
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return cert, pemBytes
}
