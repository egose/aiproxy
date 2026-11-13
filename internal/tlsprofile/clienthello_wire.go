package tlsprofile

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

const (
	MaxCaptureRecords        = 16
	MaxCaptureRecordBytes    = 18432
	MaxCaptureHandshakeBytes = 32768
)

type ClientHelloKeyShare struct {
	Group uint16 `json:"group"`
	Bytes int    `json:"bytes"`
}

type ClientHelloSummary struct {
	RecordBytes         int                   `json:"record_bytes"`
	RecordVersions      []uint16              `json:"record_versions"`
	RecordPayloadBytes  []int                 `json:"record_payload_bytes"`
	HandshakeBytes      int                   `json:"handshake_bytes"`
	HandshakeVersion    uint16                `json:"handshake_version"`
	SessionIDBytes      int                   `json:"session_id_bytes"`
	CipherSuites        []uint16              `json:"cipher_suites"`
	CompressionMethods  []int                 `json:"compression_methods"`
	ExtensionIDs        []uint16              `json:"extension_ids"`
	Groups              []uint16              `json:"groups"`
	SignatureAlgorithms []uint16              `json:"signature_algorithms"`
	ALPN                []string              `json:"alpn"`
	KeyShares           []ClientHelloKeyShare `json:"key_shares"`
}

type helloRecord struct {
	version   uint16
	length    int
	rawOffset int
	helloFrom int
}

type helloSpan struct {
	from int
	to   int
}

type helloExtension struct {
	id   uint16
	span helloSpan
}

type helloKeyShare struct {
	group uint16
	span  helloSpan
}

type capturedHello struct {
	raw        []byte
	handshake  []byte
	records    []helloRecord
	random     helloSpan
	sessionID  helloSpan
	extensions []helloExtension
	keyShares  []helloKeyShare
	summary    ClientHelloSummary
}

func ReadClientHello(reader io.Reader) ([]byte, error) {
	hello, err := readHelloRecords(reader)
	if err != nil {
		return nil, err
	}
	if err := hello.parse(); err != nil {
		return nil, err
	}
	return hello.raw, nil
}

func SummarizeClientHello(data []byte) (ClientHelloSummary, error) {
	hello, err := parseCapturedHello(data)
	if err != nil {
		return ClientHelloSummary{}, err
	}
	return hello.summary, nil
}

func NormalizeClientHello(data []byte) ([]byte, error) {
	hello, err := parseCapturedHello(data)
	if err != nil {
		return nil, err
	}
	if err := validateCapturedHello(hello, true); err != nil {
		return nil, err
	}
	if err := validateCapturedSchema(hello); err != nil {
		return nil, err
	}
	clear(hello.handshake[hello.random.from:hello.random.to])
	clear(hello.handshake[hello.sessionID.from:hello.sessionID.to])
	for _, share := range hello.keyShares {
		if !captureGREASE(share.group) {
			clear(hello.handshake[share.span.from:share.span.to])
		}
	}
	for _, record := range hello.records {
		copy(hello.raw[record.rawOffset:record.rawOffset+record.length], hello.handshake[record.helloFrom:record.helloFrom+record.length])
	}
	return hello.raw, nil
}

func readHelloRecords(reader io.Reader) (*capturedHello, error) {
	hello := &capturedHello{}
	for index := 0; index < MaxCaptureRecords; index++ {
		var header [5]byte
		if _, err := io.ReadFull(reader, header[:]); err != nil {
			return nil, fmt.Errorf("ClientHello record header: %w", err)
		}
		if header[0] != 22 {
			return nil, fmt.Errorf("ClientHello requires handshake records")
		}
		version := binary.BigEndian.Uint16(header[1:3])
		if version < 0x0301 || version > 0x0303 {
			return nil, fmt.Errorf("ClientHello record version is unsupported")
		}
		length := int(binary.BigEndian.Uint16(header[3:5]))
		if length == 0 || length > MaxCaptureRecordBytes {
			return nil, fmt.Errorf("ClientHello record payload must contain 1..18432 bytes")
		}
		if len(hello.handshake)+length > MaxCaptureHandshakeBytes {
			return nil, fmt.Errorf("ClientHello handshake exceeds 32768 bytes")
		}
		payload := make([]byte, length)
		if _, err := io.ReadFull(reader, payload); err != nil {
			return nil, fmt.Errorf("ClientHello record payload: %w", err)
		}
		hello.records = append(hello.records, helloRecord{
			version: version, length: length, rawOffset: len(hello.raw) + len(header), helloFrom: len(hello.handshake),
		})
		hello.raw = append(hello.raw, header[:]...)
		hello.raw = append(hello.raw, payload...)
		hello.handshake = append(hello.handshake, payload...)
		if hello.handshake[0] != 1 {
			return nil, fmt.Errorf("first handshake is not a ClientHello")
		}
		if len(hello.handshake) < 4 {
			continue
		}
		wanted := 4 + (int(hello.handshake[1]) << 16) + (int(hello.handshake[2]) << 8) + int(hello.handshake[3])
		if wanted > MaxCaptureHandshakeBytes {
			return nil, fmt.Errorf("ClientHello handshake exceeds 32768 bytes")
		}
		if len(hello.handshake) > wanted {
			return nil, fmt.Errorf("unexpected bytes after the first ClientHello handshake")
		}
		if len(hello.handshake) == wanted {
			return hello, nil
		}
	}
	return nil, fmt.Errorf("ClientHello exceeds the 16-record limit")
}

func parseCapturedHello(data []byte) (*capturedHello, error) {
	if len(data) > MaxCaptureHandshakeBytes+5*MaxCaptureRecords {
		return nil, fmt.Errorf("ClientHello capture exceeds its byte limit")
	}
	reader := bytes.NewReader(data)
	hello, err := readHelloRecords(reader)
	if err != nil {
		return nil, err
	}
	if reader.Len() != 0 {
		return nil, fmt.Errorf("unexpected records after the first ClientHello")
	}
	if err := hello.parse(); err != nil {
		return nil, err
	}
	return hello, nil
}

func (hello *capturedHello) parse() error {
	reader := helloReader{data: hello.handshake, offset: 4}
	version, err := reader.uint16("version")
	if err != nil {
		return err
	}
	hello.random = helloSpan{from: reader.offset, to: reader.offset + 32}
	if _, err := reader.take(32, "random"); err != nil {
		return err
	}
	if _, span, err := reader.vector8("session ID"); err != nil {
		return err
	} else if span.to-span.from > 32 {
		return fmt.Errorf("ClientHello session ID exceeds 32 bytes")
	} else {
		hello.sessionID = span
	}
	suites, _, err := reader.vector16("cipher suites")
	if err != nil {
		return err
	}
	suiteIDs, err := helloUint16List(suites, "cipher suites", 64)
	if err != nil {
		return err
	}
	compression, _, err := reader.vector8("compression methods")
	if err != nil || len(compression) == 0 {
		return fmt.Errorf("invalid ClientHello compression methods")
	}
	extensions, span, err := reader.vector16("extensions")
	if err != nil {
		return err
	}
	if reader.offset != len(reader.data) {
		return fmt.Errorf("ClientHello extension length does not match the handshake")
	}
	hello.summary = ClientHelloSummary{
		RecordBytes: len(hello.raw), HandshakeBytes: len(hello.handshake), HandshakeVersion: version,
		SessionIDBytes: hello.sessionID.to - hello.sessionID.from, CipherSuites: suiteIDs,
		CompressionMethods: make([]int, len(compression)),
	}
	for index, method := range compression {
		hello.summary.CompressionMethods[index] = int(method)
	}
	for _, record := range hello.records {
		hello.summary.RecordVersions = append(hello.summary.RecordVersions, record.version)
		hello.summary.RecordPayloadBytes = append(hello.summary.RecordPayloadBytes, record.length)
	}
	extReader := helloReader{data: extensions}
	seen := make(map[uint16]bool)
	for extReader.offset < len(extensions) {
		if len(hello.extensions) >= 32 {
			return fmt.Errorf("ClientHello exceeds the 32-extension limit")
		}
		id, err := extReader.uint16("extension type")
		if err != nil {
			return err
		}
		if seen[id] {
			return fmt.Errorf("ClientHello contains a duplicate extension")
		}
		seen[id] = true
		payload, payloadSpan, err := extReader.vector16("extension payload")
		if err != nil {
			return err
		}
		payloadSpan.from += span.from
		payloadSpan.to += span.from
		hello.extensions = append(hello.extensions, helloExtension{id: id, span: payloadSpan})
		hello.summary.ExtensionIDs = append(hello.summary.ExtensionIDs, id)
		if err := hello.parseExtension(id, payload, payloadSpan.from); err != nil {
			return err
		}
	}
	return nil
}

func (hello *capturedHello) parseExtension(id uint16, data []byte, base int) error {
	reader := helloReader{data: data}
	switch id {
	case 10, 13:
		list, _, err := reader.vector16("extension list")
		if err != nil || reader.offset != len(data) {
			return fmt.Errorf("invalid ClientHello group or signature list")
		}
		limit := 16
		if id == 13 {
			limit = 32
		}
		ids, err := helloUint16List(list, "group or signature list", limit)
		if err != nil {
			return err
		}
		if id == 10 {
			hello.summary.Groups = ids
		} else {
			hello.summary.SignatureAlgorithms = ids
		}
	case 16:
		list, _, err := reader.vector16("ALPN list")
		if err != nil || reader.offset != len(data) {
			return fmt.Errorf("invalid ClientHello ALPN list")
		}
		protocols := helloReader{data: list}
		for protocols.offset < len(list) {
			protocol, _, err := protocols.vector8("ALPN protocol")
			if err != nil || len(protocol) == 0 || len(hello.summary.ALPN) >= 2 {
				return fmt.Errorf("invalid or unsupported ClientHello ALPN protocols")
			}
			hello.summary.ALPN = append(hello.summary.ALPN, string(protocol))
		}
		if len(hello.summary.ALPN) == 0 {
			return fmt.Errorf("ClientHello ALPN list is empty")
		}
	case 51:
		list, listSpan, err := reader.vector16("key-share list")
		if err != nil || reader.offset != len(data) || len(list) == 0 {
			return fmt.Errorf("invalid ClientHello key-share list")
		}
		shares := helloReader{data: list}
		for shares.offset < len(list) {
			if len(hello.keyShares) >= 3 {
				return fmt.Errorf("ClientHello exceeds the 3-key-share limit")
			}
			group, err := shares.uint16("key-share group")
			if err != nil {
				return err
			}
			key, keySpan, err := shares.vector16("key exchange")
			if err != nil || len(key) == 0 {
				return fmt.Errorf("invalid ClientHello key exchange")
			}
			keySpan.from += base + listSpan.from
			keySpan.to += base + listSpan.from
			hello.keyShares = append(hello.keyShares, helloKeyShare{group: group, span: keySpan})
			hello.summary.KeyShares = append(hello.summary.KeyShares, ClientHelloKeyShare{Group: group, Bytes: len(key)})
		}
	}
	return nil
}

type helloReader struct {
	data   []byte
	offset int
}

func (reader *helloReader) take(length int, field string) ([]byte, error) {
	if length < 0 || length > len(reader.data)-reader.offset {
		return nil, fmt.Errorf("truncated ClientHello %s", field)
	}
	value := reader.data[reader.offset : reader.offset+length]
	reader.offset += length
	return value, nil
}

func (reader *helloReader) uint16(field string) (uint16, error) {
	value, err := reader.take(2, field)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(value), nil
}

func (reader *helloReader) vector8(field string) ([]byte, helloSpan, error) {
	length, err := reader.take(1, field+" length")
	if err != nil {
		return nil, helloSpan{}, err
	}
	from := reader.offset
	value, err := reader.take(int(length[0]), field)
	return value, helloSpan{from: from, to: reader.offset}, err
}

func (reader *helloReader) vector16(field string) ([]byte, helloSpan, error) {
	length, err := reader.uint16(field + " length")
	if err != nil {
		return nil, helloSpan{}, err
	}
	from := reader.offset
	value, err := reader.take(int(length), field)
	return value, helloSpan{from: from, to: reader.offset}, err
}

func helloUint16List(data []byte, field string, limit int) ([]uint16, error) {
	if len(data) == 0 || len(data)%2 != 0 || len(data)/2 > limit {
		return nil, fmt.Errorf("invalid or oversized ClientHello %s", field)
	}
	values := make([]uint16, 0, len(data)/2)
	for index := 0; index < len(data); index += 2 {
		values = append(values, binary.BigEndian.Uint16(data[index:index+2]))
	}
	return values, nil
}
