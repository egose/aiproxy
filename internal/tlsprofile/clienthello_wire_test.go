package tlsprofile

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func captureFixture(t testing.TB) []byte {
	t.Helper()
	encoded, err := os.ReadFile("testdata/bun-1.4.4-fetch.clienthello.hex")
	if err != nil {
		t.Fatal(err)
	}
	data, err := hex.DecodeString(strings.Join(strings.Fields(string(encoded)), ""))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 1500 || fmt.Sprintf("%x", sha256.Sum256(data)) != "a766b8a372feecaec084632ceffb7c99bd3398e21547ec347246da6b8dc7327e" {
		t.Fatal("sanitized measured ClientHello fixture changed")
	}
	return data
}

type captureChunkReader struct {
	reader io.Reader
	chunk  int
}

func (reader captureChunkReader) Read(data []byte) (int, error) {
	if len(data) > reader.chunk {
		data = data[:reader.chunk]
	}
	return reader.reader.Read(data)
}

func captureRecords(handshake []byte, sizes ...int) []byte {
	var result []byte
	for len(handshake) != 0 {
		length := len(handshake)
		if len(sizes) > 0 {
			length, sizes = sizes[0], sizes[1:]
		}
		if length > MaxCaptureRecordBytes {
			length = MaxCaptureRecordBytes
		}
		if length > len(handshake) {
			length = len(handshake)
		}
		result = append(result, 22, 3, 1, byte(length>>8), byte(length))
		result = append(result, handshake[:length]...)
		handshake = handshake[length:]
	}
	return result
}

func captureExtensionPayload(t testing.TB, raw []byte, id uint16, payload []byte, appendNew bool) []byte {
	t.Helper()
	hello, err := parseCapturedHello(raw)
	if err != nil {
		t.Fatal(err)
	}
	headerFrom := hello.extensions[0].span.from - 6
	extensionsLength := int(binary.BigEndian.Uint16(hello.handshake[headerFrom : headerFrom+2]))
	var handshake []byte
	if appendNew {
		handshake = append(handshake, hello.handshake...)
		handshake = append(handshake, byte(id>>8), byte(id), byte(len(payload)>>8), byte(len(payload)))
		handshake = append(handshake, payload...)
		extensionsLength += 4 + len(payload)
	} else {
		for _, extension := range hello.extensions {
			if extension.id != id {
				continue
			}
			handshake = append(handshake, hello.handshake[:extension.span.from]...)
			handshake = append(handshake, payload...)
			handshake = append(handshake, hello.handshake[extension.span.to:]...)
			binary.BigEndian.PutUint16(handshake[extension.span.from-2:extension.span.from], uint16(len(payload)))
			extensionsLength += len(payload) - (extension.span.to - extension.span.from)
			break
		}
		if handshake == nil {
			t.Fatal("test extension was not present")
		}
	}
	binary.BigEndian.PutUint16(handshake[headerFrom:headerFrom+2], uint16(extensionsLength))
	length := len(handshake) - 4
	handshake[1], handshake[2], handshake[3] = byte(length>>16), byte(length>>8), byte(length)
	return captureRecords(handshake)
}

func TestClientHelloReadFragmentation(t *testing.T) {
	fixture := captureFixture(t)
	for _, chunk := range []int{1, 2, 3, 5, 17, 4096, len(fixture)} {
		t.Run(fmt.Sprintf("TCP-%d", chunk), func(t *testing.T) {
			read, err := ReadClientHello(captureChunkReader{reader: bytes.NewReader(fixture), chunk: chunk})
			if err != nil || !bytes.Equal(read, fixture) {
				t.Fatalf("fragmented TCP read changed the TLS record: %v", err)
			}
		})
	}
	for _, sizes := range [][]int{{1, 1, 1, 1}, {2, 2, 17}, {17, 500, 500}, {300, 300, 300, 300}} {
		t.Run(fmt.Sprint(sizes), func(t *testing.T) {
			fragmented := captureRecords(fixture[5:], sizes...)
			read, err := ReadClientHello(captureChunkReader{reader: bytes.NewReader(fragmented), chunk: 3})
			if err != nil || !bytes.Equal(read, fragmented) {
				t.Fatalf("fragmented records were not preserved: %v", err)
			}
			summary, err := SummarizeClientHello(read)
			if err != nil || len(summary.RecordVersions) != len(sizes)+1 || summary.HandshakeBytes != 1495 {
				t.Fatalf("fragmented summary = %+v, %v", summary, err)
			}
			exported, err := ExportClientHello(fragmented)
			if err != nil {
				t.Fatal(err)
			}
			profile, err := ParseCustom(exported)
			baseline, baselineErr := ParseCustom(fixtureData(t))
			if err != nil || baselineErr != nil || profile.Identity() != baseline.Identity() {
				t.Fatal("TLS fragmentation changed the native-shaped export")
			}
		})
	}
}

func TestClientHelloReadRejectsEveryTruncation(t *testing.T) {
	fixture := captureFixture(t)
	for length := 0; length < len(fixture); length++ {
		if raw, err := ReadClientHello(bytes.NewReader(fixture[:length])); err == nil || raw != nil {
			t.Fatalf("accepted truncated capture of %d bytes", length)
		}
		if normalized, err := NormalizeClientHello(fixture[:length]); err == nil || normalized != nil {
			t.Fatalf("normalized truncated capture of %d bytes", length)
		}
	}
}

func TestClientHelloReadBounds(t *testing.T) {
	fixture := captureFixture(t)
	tests := map[string][]byte{
		"alert":               {21, 3, 1, 0, 1, 0},
		"old record version":  {22, 3, 0, 0, 1, 1},
		"new record version":  {22, 3, 4, 0, 1, 1},
		"empty record":        {22, 3, 1, 0, 0},
		"oversized record":    {22, 3, 1, 0x48, 1},
		"oversized handshake": {22, 3, 1, 0, 4, 1, 0, 0x80, 0},
		"wrong handshake":     {22, 3, 1, 0, 4, 2, 0, 0, 0},
		"empty handshake":     {22, 3, 1, 0, 4, 1, 0, 0, 0},
	}
	trailing := append([]byte(nil), fixture...)
	trailing = append(trailing, 0)
	binary.BigEndian.PutUint16(trailing[3:5], uint16(len(trailing)-5))
	tests["trailing handshake byte"] = trailing
	tests["too many records"] = captureRecords(fixture[5:], slices.Repeat([]int{1}, 16)...)
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			if raw, err := ReadClientHello(bytes.NewReader(data)); err == nil || raw != nil {
				t.Fatal("accepted malformed or oversized ClientHello")
			}
		})
	}
	extraRecord := append(append([]byte(nil), fixture...), []byte{22, 3, 1, 0, 1, 0}...)
	if _, err := NormalizeClientHello(extraRecord); err == nil {
		t.Fatal("accepted records after the first handshake")
	}
	maximum := captureExtensionPayload(t, fixture, 0xee00, make([]byte, MaxCaptureHandshakeBytes-1495-4), true)
	if read, err := ReadClientHello(bytes.NewReader(maximum)); err != nil || !bytes.Equal(read, maximum) {
		t.Fatalf("maximum bounded framing failed: %v", err)
	}
	if _, err := ExportClientHello(maximum); err == nil {
		t.Fatal("framing bounds must not make an unknown extension exportable")
	}
	oversized := captureExtensionPayload(t, fixture, 0xee00, make([]byte, MaxCaptureHandshakeBytes-1495-3), true)
	if _, err := ReadClientHello(bytes.NewReader(oversized)); err == nil {
		t.Fatal("accepted oversized reassembled handshake")
	}
	sixteen := captureRecords(fixture[5:], slices.Repeat([]int{1}, 15)...)
	if _, err := ReadClientHello(bytes.NewReader(sixteen)); err != nil {
		t.Fatalf("exact record-count bound failed: %v", err)
	}
}

func TestClientHelloMalformedVectorsAndDuplicateExtensions(t *testing.T) {
	fixture := captureFixture(t)
	hello, err := parseCapturedHello(fixture)
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string][]byte{
		"group length":        captureExtensionPayload(t, fixture, 10, []byte{0, 1, 0}, false),
		"signature length":    captureExtensionPayload(t, fixture, 13, []byte{0, 2, 4}, false),
		"ALPN empty":          captureExtensionPayload(t, fixture, 16, []byte{0, 0}, false),
		"ALPN truncated":      captureExtensionPayload(t, fixture, 16, []byte{0, 1, 2}, false),
		"key share empty":     captureExtensionPayload(t, fixture, 51, []byte{0, 0}, false),
		"key share header":    captureExtensionPayload(t, fixture, 51, []byte{0, 1, 0}, false),
		"key exchange empty":  captureExtensionPayload(t, fixture, 51, []byte{0, 4, 0, 29, 0, 0}, false),
		"duplicate extension": captureExtensionPayload(t, fixture, 35, nil, true),
	}
	sessionID := append([]byte(nil), fixture...)
	sessionID[43] = 33
	tests["session length"] = sessionID
	cipherLength := append([]byte(nil), fixture...)
	binary.BigEndian.PutUint16(cipherLength[hello.sessionID.to+5:hello.sessionID.to+7], 1)
	tests["cipher length"] = cipherLength
	compression := append([]byte(nil), fixture...)
	compression[hello.sessionID.to+2+2*len(hello.summary.CipherSuites)+5] = 0
	tests["compression empty"] = compression
	extensionsLength := append([]byte(nil), fixture...)
	from := hello.extensions[0].span.from - 6 + 5
	binary.BigEndian.PutUint16(extensionsLength[from:from+2], uint16(len(fixture)))
	tests["extensions length"] = extensionsLength
	var shares []byte
	for index := 0; index < 4; index++ {
		shares = append(shares, 0, 29, 0, 32)
		shares = append(shares, make([]byte, 32)...)
	}
	shares = append([]byte{byte(len(shares) >> 8), byte(len(shares))}, shares...)
	tests["too many shares"] = captureExtensionPayload(t, fixture, 51, shares, false)
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ReadClientHello(bytes.NewReader(raw)); err == nil {
				t.Fatal("invalid structural vectors were accepted")
			}
		})
	}
}

func TestClientHelloNormalizeOnlyVolatileBytes(t *testing.T) {
	fixture := captureFixture(t)
	hello, err := parseCapturedHello(fixture)
	if err != nil {
		t.Fatal(err)
	}
	for _, span := range append([]helloSpan{hello.random, hello.sessionID}, hello.keyShares[0].span, hello.keyShares[1].span) {
		for index := span.from; index < span.to; index++ {
			hello.handshake[index] = 0xa5
		}
	}
	for _, sizes := range [][]int{nil, {1, 3, 100, 200, 333}, {500, 500}} {
		volatile := captureRecords(hello.handshake, sizes...)
		before := append([]byte(nil), volatile...)
		normalized, err := NormalizeClientHello(volatile)
		if err != nil || !bytes.Equal(normalized, captureRecords(fixture[5:], sizes...)) {
			t.Fatalf("normalization lost framing or retained volatile data: %v", err)
		}
		if !bytes.Equal(volatile, before) {
			t.Fatal("normalization modified the caller's capture")
		}
		second, err := NormalizeClientHello(normalized)
		if err != nil || !bytes.Equal(normalized, second) {
			t.Fatal("normalization is not idempotent")
		}
	}
}

func TestClientHelloNormalizeRetainsNonvolatileStructure(t *testing.T) {
	fixture := captureFixture(t)
	mutations := map[string]func([]byte) []byte{
		"record version":                func(raw []byte) []byte { raw[2] = 3; return raw },
		"record lengths and boundaries": func(raw []byte) []byte { return captureRecords(raw[5:], 17, 300) },
		"cipher order": func(raw []byte) []byte {
			hello, _ := parseCapturedHello(raw)
			from := hello.sessionID.to + 2 + 5
			raw[from], raw[from+2] = raw[from+2], raw[from]
			raw[from+1], raw[from+3] = raw[from+3], raw[from+1]
			return raw
		},
		"extension order": func(raw []byte) []byte {
			hello, _ := parseCapturedHello(raw)
			first, second := hello.extensions[0], hello.extensions[1]
			handshake := append([]byte(nil), hello.handshake[:first.span.from-4]...)
			handshake = append(handshake, hello.handshake[second.span.from-4:second.span.to]...)
			handshake = append(handshake, hello.handshake[first.span.from-4:first.span.to]...)
			handshake = append(handshake, hello.handshake[second.span.to:]...)
			return captureRecords(handshake)
		},
		"groups order": func(raw []byte) []byte {
			return captureExtensionPayload(t, raw, 10, []byte{0, 8, 0x11, 0xec, 0, 29, 0, 24, 0, 23}, false)
		},
		"signature order": func(raw []byte) []byte {
			hello, _ := parseCapturedHello(raw)
			for _, ext := range hello.extensions {
				if ext.id == 13 {
					from := 5 + ext.span.from + 2
					raw[from], raw[from+2] = raw[from+2], raw[from]
					raw[from+1], raw[from+3] = raw[from+3], raw[from+1]
				}
			}
			return raw
		},
		"ALPN": func(raw []byte) []byte {
			return captureExtensionPayload(t, raw, 16, []byte{0, 12, 2, 'h', '2', 8, 'h', 't', 't', 'p', '/', '1', '.', '1'}, false)
		},
		"SNI":     func(raw []byte) []byte { return bytes.Replace(raw, []byte("localhost"), []byte("localtest"), 1) },
		"padding": func(raw []byte) []byte { return captureExtensionPayload(t, raw, 21, make([]byte, 8), true) },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			normalized, err := NormalizeClientHello(mutate(append([]byte(nil), fixture...)))
			if err != nil || bytes.Equal(fixture, normalized) {
				t.Fatalf("nonvolatile structure was erased: %v", err)
			}
		})
	}
	oldHandshake := append([]byte(nil), fixture...)
	oldHandshake[10] = 2
	if _, err := NormalizeClientHello(oldHandshake); err == nil {
		t.Fatal("unsupported handshake versions must not be normalized into a match")
	}
	badShare := captureExtensionPayload(t, fixture, 51, []byte{0, 5, 0, 29, 0, 1, 0}, false)
	if _, err := NormalizeClientHello(badShare); err == nil {
		t.Fatal("invalid key-share lengths must not be cleared into a match")
	}
}

func TestClientHelloSummaryIsStructuralAndIndependent(t *testing.T) {
	fixture := captureFixture(t)
	summary, err := SummarizeClientHello(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if summary.RecordBytes != 1500 || summary.HandshakeBytes != 1495 || summary.HandshakeVersion != 771 || summary.SessionIDBytes != 32 ||
		!slices.Equal(summary.RecordVersions, []uint16{769}) || !slices.Equal(summary.RecordPayloadBytes, []int{1495}) ||
		!slices.Equal(summary.Groups, []uint16{4588, 29, 23, 24}) || !slices.Equal(summary.ALPN, []string{"http/1.1"}) ||
		!reflect.DeepEqual(summary.KeyShares, []ClientHelloKeyShare{{Group: 4588, Bytes: 1216}, {Group: 29, Bytes: 32}}) {
		t.Fatalf("unexpected measured structure: %+v", summary)
	}
	summary.CipherSuites[0] = 0
	summary.Groups[0] = 0
	clear(fixture)
	fresh, err := SummarizeClientHello(captureFixture(t))
	if err != nil || fresh.CipherSuites[0] != 4865 || fresh.Groups[0] != 4588 {
		t.Fatal("summary exposed shared or caller-owned storage")
	}
}

func FuzzClientHelloCapture(f *testing.F) {
	f.Add(captureFixture(f))
	f.Add([]byte{22, 3, 1, 0, 4, 1, 0, 0, 0})
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		normalized, err := NormalizeClientHello(data)
		if err == nil {
			if len(normalized) != len(data) {
				t.Fatal("normalization changed capture length")
			}
			second, err := NormalizeClientHello(normalized)
			if err != nil || !bytes.Equal(normalized, second) {
				t.Fatal("normalization was not idempotent")
			}
		}
		exported, err := ExportClientHello(data)
		if err == nil {
			if _, err := ParseCustom(exported); err != nil {
				t.Fatal("export was outside the accepted schema")
			}
		}
	})
}
