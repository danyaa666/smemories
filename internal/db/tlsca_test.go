package db

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

// testCA returns a self-signed CA written to a PEM file, and a server certificate for 127.0.0.1 signed by it.
func testCA(t *testing.T) (caFile string, server tls.Certificate) {
	t.Helper()
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test ca"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	caDER, err := x509.CreateCertificate(rand.Reader, caTpl, caTpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caFile = filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	ca, _ := x509.ParseCertificate(caDER)
	srvKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	srvTpl := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "db"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	srvDER, err := x509.CreateCertificate(rand.Reader, srvTpl, ca, &srvKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	return caFile, tls.Certificate{Certificate: [][]byte{srvDER}, PrivateKey: srvKey}
}

func TestUseCAFileVerifiesAgainstTheBundle(t *testing.T) {
	caFile, cert := testCA(t)
	otherCA, _ := testCA(t)
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { _ = c.(*tls.Conn).Handshake(); _ = c.Close() }()
		}
	}()
	handshake := func(bundle string) error {
		mc := &mysql.Config{Addr: ln.Addr().String()}
		if err := UseCAFile(mc, bundle); err != nil {
			t.Fatal(err)
		}
		c, err := tls.Dial("tcp", mc.Addr, mc.TLS)
		if err == nil {
			_ = c.Close()
		}
		return err
	}
	if err := handshake(caFile); err != nil {
		t.Fatalf("certificate signed by the bundle's CA: %v", err)
	}
	if err := handshake(otherCA); err == nil {
		t.Fatal("a certificate signed by another CA was accepted")
	}
}

func TestUseCAFileErrors(t *testing.T) {
	junk := filepath.Join(t.TempDir(), "junk.pem")
	if err := os.WriteFile(junk, []byte("not pem"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{junk, "/nonexistent/ca.pem"} {
		if err := UseCAFile(&mysql.Config{Addr: "h:3306"}, p); err == nil || !strings.Contains(err.Error(), "SMEM_DB_TLS_CA_FILE") {
			t.Errorf("%s: want an error naming SMEM_DB_TLS_CA_FILE, got %v", p, err)
		}
	}
}
