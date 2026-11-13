package upstreamhttp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"testing"

	utls "github.com/refraction-networking/utls"
)

func guardHandshake(kind byte, body []byte) []byte {
	length := len(body)
	return append([]byte{kind, byte(length >> 16), byte(length >> 8), byte(length)}, body...)
}

func guardRecord(kind byte, body []byte) []byte {
	return append([]byte{kind, 3, 3, byte(len(body) >> 8), byte(len(body))}, body...)
}

func guardServerHello(version, cipher uint16) []byte {
	body := make([]byte, 38)
	binary.BigEndian.PutUint16(body[:2], version)
	binary.BigEndian.PutUint16(body[35:37], cipher)
	if version == utls.VersionTLS13 {
		binary.BigEndian.PutUint16(body[:2], utls.VersionTLS12)
		body = append(body, 0, 6, 0, 43, 0, 2, 3, 4)
	}
	return guardHandshake(2, body)
}

func guardServerKeyExchange(group, scheme uint16) []byte {
	body := []byte{3, byte(group >> 8), byte(group), 65}
	body = append(body, make([]byte, 65)...)
	body = append(body, byte(scheme>>8), byte(scheme), 0, 1, 0)
	return guardHandshake(12, body)
}

func guardFragmentRecords(handshake []byte, recordSize int) []byte {
	var records []byte
	for len(handshake) > 0 {
		n := min(recordSize, len(handshake))
		records = append(records, guardRecord(22, handshake[:n])...)
		handshake = handshake[n:]
	}
	return records
}

func TestServerNegotiationGuardFragmentationAndCertificateSkipping(t *testing.T) {
	for _, scheme := range []uint16{uint16(utls.ECDSAWithP256AndSHA256), uint16(utls.ECDSAWithSHA1)} {
		hello := guardServerHello(utls.VersionTLS12, utls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256)
		certificate := guardHandshake(11, make([]byte, 128<<10))
		exchange := guardServerKeyExchange(uint16(utls.CurveP256), scheme)
		for _, recordSize := range []int{1, 3, 137, maxInitialTLSRecordBytes} {
			records := guardFragmentRecords(append(append(hello, certificate...), exchange...), recordSize)
			for _, chunk := range []int{1, 2, 3, 5, 17, 4096, len(records)} {
				var guard serverNegotiationGuard
				var err error
				for pos := 0; pos < len(records); pos += chunk {
					err = guard.feed(records[pos:min(pos+chunk, len(records))])
					if len(guard.messageBody) > maxServerHelloBytes || (guard.messageType == 11 && guard.messageBody != nil) {
						t.Fatal("certificate payload was buffered")
					}
					if err != nil {
						break
					}
				}
				if scheme == uint16(utls.ECDSAWithP256AndSHA256) && (err != nil || guard.finish() != nil) {
					t.Fatalf("valid fragmented negotiation record=%d tcp=%d: %v", recordSize, chunk, err)
				}
				if scheme == uint16(utls.ECDSAWithSHA1) && err == nil {
					t.Fatalf("fragmented SHA-1 bypass record=%d tcp=%d", recordSize, chunk)
				}
			}
		}
	}
}

func TestServerNegotiationGuardWarningAlertsCannotBypass(t *testing.T) {
	hello := guardRecord(22, guardServerHello(utls.VersionTLS12, utls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256))
	weak := guardRecord(22, guardServerKeyExchange(uint16(utls.X25519), uint16(utls.PKCS1WithSHA1)))
	warning := guardRecord(21, []byte{1, 90})
	for _, input := range [][]byte{
		append(append(warning, hello...), weak...),
		append(append(hello, warning...), weak...),
	} {
		var guard serverNegotiationGuard
		if err := guard.feed(input); err == nil {
			t.Fatal("warning alert disabled negotiation inspection")
		}
	}
}

func TestServerNegotiationGuardSafeSelections(t *testing.T) {
	for _, version := range []uint16{utls.VersionTLS12, utls.VersionTLS13} {
		for _, cipher := range []uint16{
			utls.TLS_AES_128_GCM_SHA256, utls.TLS_AES_256_GCM_SHA384, utls.TLS_CHACHA20_POLY1305_SHA256,
			utls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256, utls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			utls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, utls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			utls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256, utls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
		} {
			var guard serverNegotiationGuard
			err := guard.feed(guardRecord(22, guardServerHello(version, cipher)))
			valid := (version == utls.VersionTLS13) == (cipher >= 0x1301 && cipher <= 0x1303)
			if (err == nil) != valid {
				t.Fatalf("version/cipher mismatch accepted: %x %x %v", version, cipher, err)
			}
		}
	}
	for _, group := range []utls.CurveID{utls.X25519, utls.CurveP256, utls.CurveP384, utls.CurveP521} {
		for _, scheme := range []utls.SignatureScheme{
			utls.PSSWithSHA256, utls.PSSWithSHA384, utls.PSSWithSHA512,
			utls.PKCS1WithSHA256, utls.PKCS1WithSHA384, utls.PKCS1WithSHA512,
			utls.ECDSAWithP256AndSHA256, utls.ECDSAWithP384AndSHA384, utls.ECDSAWithP521AndSHA512, utls.Ed25519,
		} {
			var guard serverNegotiationGuard
			input := append(guardServerHello(utls.VersionTLS12, utls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256), guardServerKeyExchange(uint16(group), uint16(scheme))...)
			if err := guard.feed(guardRecord(22, input)); err != nil || guard.finish() != nil {
				t.Fatalf("safe group/signature rejected: %d %x %v", group, scheme, err)
			}
		}
	}
}

func TestServerNegotiationGuardWeakSelections(t *testing.T) {
	for _, version := range []uint16{utls.VersionSSL30, utls.VersionTLS10, utls.VersionTLS11, 0x0305} {
		var guard serverNegotiationGuard
		if err := guard.feed(guardRecord(22, guardServerHello(version, utls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256))); err == nil {
			t.Fatalf("weak version accepted: %x", version)
		}
	}
	for _, cipher := range []uint16{0, utls.TLS_RSA_WITH_AES_128_GCM_SHA256, utls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA, utls.TLS_RSA_WITH_3DES_EDE_CBC_SHA, 0xffff} {
		var guard serverNegotiationGuard
		if err := guard.feed(guardRecord(22, guardServerHello(utls.VersionTLS12, cipher))); err == nil {
			t.Fatalf("weak cipher accepted: %x", cipher)
		}
	}
	for _, group := range []uint16{0, 1, 0x0100, uint16(utls.X25519MLKEM768), 0xffff} {
		var guard serverNegotiationGuard
		input := append(guardServerHello(utls.VersionTLS12, utls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256), guardServerKeyExchange(group, uint16(utls.PSSWithSHA256))...)
		if err := guard.feed(guardRecord(22, input)); err == nil {
			t.Fatalf("unsafe TLS 1.2 group accepted: %d", group)
		}
	}
	for _, scheme := range []uint16{0, uint16(utls.PKCS1WithSHA1), uint16(utls.ECDSAWithSHA1), 0x0202, 0x0303, 0x0808, 0xffff} {
		var guard serverNegotiationGuard
		input := append(guardServerHello(utls.VersionTLS12, utls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256), guardServerKeyExchange(uint16(utls.X25519), scheme)...)
		if err := guard.feed(guardRecord(22, input)); err == nil {
			t.Fatalf("weak TLS 1.2 signature accepted: %x", scheme)
		}
	}
}

func TestServerNegotiationGuardMalformedAndBounds(t *testing.T) {
	hello := guardServerHello(utls.VersionTLS12, utls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256)
	exchange := guardServerKeyExchange(uint16(utls.CurveP256), uint16(utls.ECDSAWithP256AndSHA256))
	badHello := func(change func([]byte) []byte) []byte {
		return guardRecord(22, guardHandshake(2, change(append([]byte(nil), hello[4:]...))))
	}
	badExchange := func(change func([]byte) []byte) []byte {
		return guardRecord(22, append(hello, guardHandshake(12, change(append([]byte(nil), exchange[4:]...)))...))
	}
	inputs := [][]byte{
		{22, 3, 3, 0, 0},
		{22, 3, 3, 0xff, 0xff},
		{22, 2, 3, 0, 1},
		{22, 3, 4, 0, 1},
		guardRecord(20, []byte{1}),
		guardRecord(23, []byte{1}),
		guardRecord(21, []byte{1}),
		guardRecord(22, []byte{2, 1, 0, 1}),
		guardRecord(22, append(hello, []byte{12, 0, 0x10, 1}...)),
		guardRecord(22, exchange),
		guardRecord(22, guardHandshake(2, make([]byte, 10))),
		guardRecord(22, append(hello, hello...)),
		guardRecord(22, append(hello, guardHandshake(14, nil)...)),
		badHello(func(body []byte) []byte { body[34] = 33; return body }),
		badHello(func(body []byte) []byte { body[34] = 32; return body }),
		badHello(func(body []byte) []byte { body[37] = 1; return body }),
		badHello(func(body []byte) []byte { return append(body, 0) }),
		badHello(func(body []byte) []byte { return append(body, 0, 4, 0, 43, 0, 2) }),
		badHello(func(body []byte) []byte { return append(body, 0, 1, 0) }),
		badHello(func(body []byte) []byte { return append(body, 0, 5, 0, 43, 0, 1, 3) }),
		badHello(func(body []byte) []byte { return append(body, 0, 6, 0, 43, 0, 2, 3, 3) }),
		badHello(func(body []byte) []byte { return append(body, 0, 12, 0, 43, 0, 2, 3, 4, 0, 43, 0, 2, 3, 4) }),
		badExchange(func(body []byte) []byte { body[0] = 1; return body }),
		badExchange(func(body []byte) []byte { body[3] = 0; return body }),
		badExchange(func(body []byte) []byte { body[3] = 255; return body }),
		badExchange(func(body []byte) []byte { return body[:len(body)-1] }),
		badExchange(func(body []byte) []byte { body[len(body)-2] = 0; return body }),
		badExchange(func(body []byte) []byte { return append(body, 0) }),
	}
	for i, input := range inputs {
		var guard serverNegotiationGuard
		if err := guard.feed(input); err == nil {
			t.Fatalf("malformed case %d accepted", i)
		} else if again := guard.feed(guardRecord(22, guardServerHello(utls.VersionTLS13, utls.TLS_AES_128_GCM_SHA256))); !errors.Is(again, err) {
			t.Fatal("guard error was not sticky")
		}
	}
	valid := guardRecord(22, guardServerHello(utls.VersionTLS13, utls.TLS_AES_128_GCM_SHA256))
	for end := 0; end < len(valid); end++ {
		var guard serverNegotiationGuard
		if err := guard.feed(valid[:end]); err != nil || guard.finish() == nil {
			t.Fatalf("truncated handshake boundary %d: %v", end, err)
		}
	}
}

func TestServerNegotiationGuardLeavesTLS13AndValidatedTLS12BytesUntouched(t *testing.T) {
	for _, handshake := range [][]byte{
		guardServerHello(utls.VersionTLS13, utls.TLS_AES_128_GCM_SHA256),
		append(guardServerHello(utls.VersionTLS12, utls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256), guardServerKeyExchange(uint16(utls.X25519), uint16(utls.PSSWithSHA256))...),
	} {
		wire := append(guardFragmentRecords(handshake, 7), []byte{23, 3, 3, 0, 3, 1, 2, 3}...)
		conn := &negotiationGuardConn{Conn: &guardReaderConn{Reader: bytes.NewReader(wire)}}
		got, err := io.ReadAll(conn)
		if err != nil || !bytes.Equal(got, wire) || conn.guard.finish() != nil {
			t.Fatalf("guard changed validated TLS bytes: %v", err)
		}
	}
	conn := &negotiationGuardConn{Conn: &guardReaderConn{Reader: bytes.NewReader([]byte{22, 3})}}
	if _, err := io.ReadAll(conn); err == nil {
		t.Fatal("truncated initial TLS read succeeded")
	}
}

type guardReaderConn struct {
	net.Conn
	io.Reader
}

func (c *guardReaderConn) Read(p []byte) (int, error) { return c.Reader.Read(p) }

func FuzzServerNegotiationGuard(f *testing.F) {
	f.Add(guardRecord(22, guardServerHello(utls.VersionTLS13, utls.TLS_AES_128_GCM_SHA256)), uint16(1))
	f.Add(guardRecord(22, append(guardServerHello(utls.VersionTLS12, utls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256), guardServerKeyExchange(uint16(utls.CurveP256), uint16(utls.PKCS1WithSHA1))...)), uint16(3))
	f.Add([]byte{22, 3, 3, 0xff, 0xff}, uint16(17))
	f.Fuzz(func(t *testing.T, data []byte, chunk uint16) {
		if len(data) > 1<<20 {
			t.Skip()
		}
		var whole, fragmented serverNegotiationGuard
		wholeErr := whole.feed(data)
		var fragmentedErr error
		step := 1 + int(chunk)%4096
		for pos := 0; pos < len(data); pos += step {
			fragmentedErr = fragmented.feed(data[pos:min(pos+step, len(data))])
			if len(fragmented.messageBody) > maxServerHelloBytes || (fragmented.messageType == 12 && len(fragmented.messageBody) > maxServerKeyExchangeBytes) || ((fragmented.messageType == 11 || fragmented.messageType == 22) && fragmented.messageBody != nil) {
				t.Fatal("guard exceeded a buffer bound")
			}
			if fragmentedErr != nil {
				break
			}
		}
		if (wholeErr == nil) != (fragmentedErr == nil) || whole.done != fragmented.done || whole.selectedVersion != fragmented.selectedVersion || (whole.finish() == nil) != (fragmented.finish() == nil) {
			t.Fatal("fragmentation changed guard result")
		}
		if wholeErr != nil && !errors.Is(whole.feed(nil), wholeErr) {
			t.Fatal("guard error was not sticky")
		}
	})
}
