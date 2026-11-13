package upstreamhttp

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"

	utls "github.com/refraction-networking/utls"
)

const (
	maxInitialTLSRecordBytes  = 18432
	maxServerHelloBytes       = 65536
	maxServerKeyExchangeBytes = 4096
)

type negotiationGuardConn struct {
	net.Conn
	guard serverNegotiationGuard
}

func (c *negotiationGuardConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 {
		if policyErr := c.guard.feed(p[:n]); policyErr != nil {
			return 0, policyErr
		}
	}
	if err == io.EOF {
		if policyErr := c.guard.finish(); policyErr != nil {
			return n, policyErr
		}
	}
	return n, err
}

type serverNegotiationGuard struct {
	recordHeader       [5]byte
	recordHeaderBytes  int
	recordRemaining    int
	recordType         byte
	messageHeader      [4]byte
	messageHeaderBytes int
	messageRemaining   int
	messageType        byte
	messageBody        []byte
	selectedVersion    uint16
	done               bool
	err                error
}

func (g *serverNegotiationGuard) feed(data []byte) (err error) {
	if g.err != nil {
		return g.err
	}
	defer func() {
		if err != nil {
			g.err = err
		}
	}()
	for len(data) > 0 && !g.done {
		if g.recordRemaining == 0 {
			n := min(len(g.recordHeader)-g.recordHeaderBytes, len(data))
			copy(g.recordHeader[g.recordHeaderBytes:], data[:n])
			g.recordHeaderBytes += n
			data = data[n:]
			if g.recordHeaderBytes != len(g.recordHeader) {
				continue
			}
			g.recordType = g.recordHeader[0]
			g.recordRemaining = int(binary.BigEndian.Uint16(g.recordHeader[3:]))
			g.recordHeaderBytes = 0
			if g.recordHeader[1] != 3 || g.recordHeader[2] > 3 || g.recordRemaining == 0 || g.recordRemaining > maxInitialTLSRecordBytes {
				return errors.New("invalid initial server TLS record")
			}
			if g.recordType != 22 && g.recordType != 21 {
				return errors.New("unexpected TLS record before validated upstream negotiation")
			}
			if g.recordType == 21 && g.recordRemaining != 2 {
				return errors.New("invalid initial server TLS alert")
			}
		}
		n := min(g.recordRemaining, len(data))
		if g.recordType == 22 {
			if err := g.feedHandshake(data[:n]); err != nil {
				return err
			}
		}
		data = data[n:]
		g.recordRemaining -= n
	}
	return nil
}

func (g *serverNegotiationGuard) feedHandshake(data []byte) error {
	for len(data) > 0 && !g.done {
		if g.messageRemaining == 0 {
			n := min(len(g.messageHeader)-g.messageHeaderBytes, len(data))
			copy(g.messageHeader[g.messageHeaderBytes:], data[:n])
			g.messageHeaderBytes += n
			data = data[n:]
			if g.messageHeaderBytes != len(g.messageHeader) {
				continue
			}
			g.messageType = g.messageHeader[0]
			g.messageRemaining = int(g.messageHeader[1])<<16 | int(g.messageHeader[2])<<8 | int(g.messageHeader[3])
			g.messageHeaderBytes = 0
			g.messageBody = nil
			if g.selectedVersion == 0 && g.messageType != 2 {
				return errors.New("expected initial upstream TLS ServerHello")
			}
			switch g.messageType {
			case 2:
				if g.selectedVersion != 0 {
					return errors.New("unexpected repeated upstream TLS ServerHello")
				}
				if g.messageRemaining > maxServerHelloBytes {
					return errors.New("oversized upstream TLS ServerHello")
				}
				g.messageBody = make([]byte, 0, g.messageRemaining)
			case 12:
				if g.messageRemaining > maxServerKeyExchangeBytes {
					return errors.New("oversized upstream TLS ServerKeyExchange")
				}
				g.messageBody = make([]byte, 0, g.messageRemaining)
			case 11, 22:
			default:
				return errors.New("unexpected handshake before validated upstream TLS key exchange")
			}
			if g.messageRemaining == 0 {
				if err := g.messageComplete(); err != nil {
					return err
				}
				continue
			}
		}
		n := min(g.messageRemaining, len(data))
		if g.messageType == 2 || g.messageType == 12 {
			g.messageBody = append(g.messageBody, data[:n]...)
		}
		data = data[n:]
		g.messageRemaining -= n
		if g.messageRemaining == 0 {
			if err := g.messageComplete(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (g *serverNegotiationGuard) messageComplete() error {
	switch g.messageType {
	case 2:
		version, cipher, err := parseServerHello(g.messageBody)
		if err != nil {
			return err
		}
		if err := verifyTLSSelection(version, cipher); err != nil {
			return err
		}
		g.selectedVersion = version
		g.done = version == utls.VersionTLS13
	case 12:
		if g.selectedVersion != utls.VersionTLS12 {
			return errors.New("ServerKeyExchange before upstream TLS 1.2 ServerHello")
		}
		if err := verifyServerKeyExchange(g.messageBody); err != nil {
			return err
		}
		g.done = true
	}
	g.messageBody = nil
	return nil
}

func (g *serverNegotiationGuard) finish() error {
	if g.err != nil {
		return g.err
	}
	if !g.done {
		return errors.New("incomplete validated upstream TLS negotiation")
	}
	return nil
}

func parseServerHello(body []byte) (uint16, uint16, error) {
	if len(body) < 38 || body[34] > 32 {
		return 0, 0, errors.New("malformed upstream TLS ServerHello")
	}
	legacyVersion := binary.BigEndian.Uint16(body[:2])
	version := legacyVersion
	pos := 35 + int(body[34])
	if pos+3 > len(body) || body[pos+2] != 0 {
		return 0, 0, errors.New("malformed upstream TLS ServerHello session or compression")
	}
	cipher := binary.BigEndian.Uint16(body[pos : pos+2])
	pos += 3
	if pos < len(body) {
		if pos+2 > len(body) || pos+2+int(binary.BigEndian.Uint16(body[pos:pos+2])) != len(body) {
			return 0, 0, errors.New("malformed upstream TLS ServerHello extensions")
		}
		pos += 2
		seenVersion := false
		for pos < len(body) {
			if pos+4 > len(body) {
				return 0, 0, errors.New("malformed upstream TLS ServerHello extension header")
			}
			id := binary.BigEndian.Uint16(body[pos : pos+2])
			n := int(binary.BigEndian.Uint16(body[pos+2 : pos+4]))
			pos += 4
			if pos+n > len(body) {
				return 0, 0, errors.New("malformed upstream TLS ServerHello extension payload")
			}
			if id == 43 {
				if seenVersion || n != 2 || legacyVersion != utls.VersionTLS12 {
					return 0, 0, errors.New("malformed selected upstream TLS supported version")
				}
				seenVersion = true
				version = binary.BigEndian.Uint16(body[pos : pos+2])
				if version != utls.VersionTLS13 {
					return 0, 0, errors.New("unsafe selected upstream TLS supported version")
				}
			}
			pos += n
		}
	}
	return version, cipher, nil
}

func verifyServerKeyExchange(body []byte) error {
	if len(body) < 8 || body[0] != 3 || body[3] == 0 {
		return errors.New("unsupported upstream TLS 1.2 ServerKeyExchange key agreement")
	}
	group := utls.CurveID(binary.BigEndian.Uint16(body[1:3]))
	switch group {
	case utls.X25519, utls.CurveP256, utls.CurveP384, utls.CurveP521:
	default:
		return fmt.Errorf("unsafe selected upstream TLS 1.2 group %d", group)
	}
	pos := 4 + int(body[3])
	if pos+4 > len(body) {
		return errors.New("malformed upstream TLS ServerKeyExchange signature")
	}
	scheme := utls.SignatureScheme(binary.BigEndian.Uint16(body[pos : pos+2]))
	switch scheme {
	case utls.PSSWithSHA256, utls.PSSWithSHA384, utls.PSSWithSHA512,
		utls.PKCS1WithSHA256, utls.PKCS1WithSHA384, utls.PKCS1WithSHA512,
		utls.ECDSAWithP256AndSHA256, utls.ECDSAWithP384AndSHA384, utls.ECDSAWithP521AndSHA512, utls.Ed25519:
	default:
		return fmt.Errorf("unsafe selected upstream TLS 1.2 signature 0x%04x", uint16(scheme))
	}
	length := int(binary.BigEndian.Uint16(body[pos+2 : pos+4]))
	if length == 0 || pos+4+length != len(body) {
		return errors.New("malformed upstream TLS ServerKeyExchange signature length")
	}
	return nil
}
