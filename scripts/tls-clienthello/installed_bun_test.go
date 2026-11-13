package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"math/big"
	"net"
	"os"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/tlsprofile"
	utls "github.com/refraction-networking/utls"
)

func TestInstalledBunCaptureCompareAndVerifiedTLS(t *testing.T) {
	selected := os.Getenv("TLS_CLIENTHELLO_TEST_BUN")
	if selected == "" {
		t.Skip("set TLS_CLIENTHELLO_TEST_BUN to an already-installed Bun executable for the optional real-runtime test")
	}
	executable, err := resolveBun(context.Background(), selected)
	if err != nil {
		t.Fatal(err)
	}
	runner := bunCapture{executable: executable, limits: defaultCaptureLimits()}
	var captures [][]byte
	var profile tlsprofile.Profile
	for sample := 0; sample < 3; sample++ {
		raw, info, err := runner.capture(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if info.Version != "1.4.4" {
			t.Fatal("this measured-fixture smoke test requires Bun 1.4.4")
		}
		exported, err := tlsprofile.ExportClientHello(raw)
		if err != nil {
			t.Fatal(err)
		}
		profile, err = tlsprofile.ParseCustom(exported)
		if err != nil || !bytes.Equal(profile.CanonicalJSON(), toolFixtureProfile(t).CanonicalJSON()) {
			t.Fatalf("installed Bun capture was not accepted as the measured recipe: %v", err)
		}
		captures = append(captures, raw)
	}
	comparison, err := compareProfile(context.Background(), captures, profile, defaultCaptureLimits())
	if err != nil || !comparison.Matched || comparison.Structure.RecordBytes != 1500 {
		t.Fatalf("real Bun/uTLS comparison failed: %+v, %v", comparison, err)
	}
	t.Log("three fresh Bun 1.4.4 fetch and three fresh materialized uTLS captures match after random/session-ID/real-key-byte normalization")
	certificate, roots := installedSmokeCertificate(t)
	for _, version := range []uint16{tls.VersionTLS12, tls.VersionTLS13} {
		t.Run(map[uint16]string{tls.VersionTLS12: "TLS1.2", tls.VersionTLS13: "TLS1.3"}[version], func(t *testing.T) {
			verifyCapturedProfileLocally(t, profile, certificate, roots, version)
		})
	}
}

func installedSmokeCertificate(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), DNSNames: []string{"localhost"},
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true, IsCA: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(parsed)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, roots
}

func verifyCapturedProfileLocally(t *testing.T, profile tlsprofile.Profile, certificate tls.Certificate, roots *x509.CertPool, version uint16) {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan error, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer connection.Close()
		connection.SetDeadline(time.Now().Add(3 * time.Second))
		server := tls.Server(connection, &tls.Config{
			Certificates: []tls.Certificate{certificate}, MinVersion: version, MaxVersion: version,
			NextProtos: []string{"http/1.1"}, CipherSuites: []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256},
		})
		done <- server.Handshake()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	connection, err := (&net.Dialer{}).DialContext(ctx, "tcp4", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	spec, err := profile.Materialize()
	if err != nil {
		t.Fatal(err)
	}
	client := utls.UClient(connection, &utls.Config{ServerName: "localhost", RootCAs: roots}, utls.HelloCustom)
	if err := client.ApplyPreset(spec); err != nil {
		t.Fatal(err)
	}
	err = client.HandshakeContext(ctx)
	connection.Close()
	serverErr := <-done
	if err != nil || serverErr != nil {
		t.Fatalf("measured profile failed verified local TLS: client %v, server %v", err, serverErr)
	}
	state := client.ConnectionState()
	if state.Version != version || state.NegotiatedProtocol != "http/1.1" || len(state.VerifiedChains) == 0 {
		t.Fatal("measured profile was not a successful verified TLS connection")
	}
}
