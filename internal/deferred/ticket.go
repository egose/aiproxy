package deferred

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
)

const ticketPayloadBytes = 157
const ticketBytes = ticketPayloadBytes + sha256.Size

type submitScope struct {
	untilMS int64
	limits  Limits
	digest  string
}

func randomID(random io.Reader) (string, error) {
	var value [16]byte
	if _, err := io.ReadFull(random, value[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value[:]), nil
}

func NewAttemptKey() (string, error) {
	return randomID(rand.Reader)
}

func hashFields(fields ...string) [sha256.Size]byte {
	h := sha256.New()
	for _, field := range fields {
		hashField(h, []byte(field))
	}
	var out [sha256.Size]byte
	copy(out[:], h.Sum(nil))
	return out
}

func hashField(h hash.Hash, b []byte) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(b)))
	_, _ = h.Write(size[:])
	_, _ = h.Write(b)
}

func ownerDigest(owner Owner) [sha256.Size]byte {
	return hashFields(owner.Kind, owner.ID, owner.Tenant, owner.Workspace)
}

func Fingerprint(publicModel string, body []byte, headers http.Header) (string, error) {
	if err := ValidatePublicModel(publicModel); err != nil {
		return "", err
	}
	canonical, err := CanonicalSubmitHeaders(headers)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	hashField(h, []byte("POST /v1/chat/completions"))
	hashField(h, []byte(publicModel))
	hashField(h, body)
	keys := make([]string, 0, len(canonical))
	for name := range canonical {
		keys = append(keys, strings.ToLower(name))
	}
	sort.Strings(keys)
	var count [8]byte
	binary.BigEndian.PutUint64(count[:], uint64(len(keys)))
	_, _ = h.Write(count[:])
	for _, name := range keys {
		hashField(h, []byte(name))
		values := canonical[http.CanonicalHeaderKey(name)]
		binary.BigEndian.PutUint64(count[:], uint64(len(values)))
		_, _ = h.Write(count[:])
		for _, value := range values {
			hashField(h, []byte(value))
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (s *Store) issueTicket(owner Owner, model string, limits Limits, untilMS int64) (string, error) {
	var raw [ticketBytes]byte
	raw[0] = Version
	generation, _ := base64.RawURLEncoding.DecodeString(s.generation)
	copy(raw[1:17], generation)
	ownerHash, modelHash := ownerDigest(owner), hashFields(model)
	copy(raw[17:49], ownerHash[:])
	copy(raw[49:81], modelHash[:])
	if _, err := io.ReadFull(s.random, raw[81:97]); err != nil {
		return "", err
	}
	binary.BigEndian.PutUint64(raw[97:105], uint64(untilMS))
	for i, value := range []int64{
		limits.RequestBytes, limits.ResultBytes, limits.ExecutionTimeoutMS,
		limits.RetentionMS, limits.ControlTimeoutMS, limits.FinalizationTimeoutMS,
	} {
		binary.BigEndian.PutUint64(raw[105+i*8:113+i*8], uint64(value))
	}
	binary.BigEndian.PutUint32(raw[153:157], uint32(limits.ReadBytes))
	mac := hmac.New(sha256.New, s.secret[:])
	_, _ = mac.Write(raw[:ticketPayloadBytes])
	copy(raw[ticketPayloadBytes:], mac.Sum(nil))
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func (s *Store) verifyTicket(owner Owner, model, token string) (submitScope, error) {
	if len(token) != base64.RawURLEncoding.EncodedLen(ticketBytes) || len(token) > MaxSubmitTokenBytes {
		return submitScope{}, fmt.Errorf("invalid submission ticket")
	}
	var raw [ticketBytes]byte
	n, err := base64.RawURLEncoding.Strict().Decode(raw[:], []byte(token))
	if err != nil || n != len(raw) || base64.RawURLEncoding.EncodeToString(raw[:]) != token {
		return submitScope{}, fmt.Errorf("invalid submission ticket")
	}
	mac := hmac.New(sha256.New, s.secret[:])
	_, _ = mac.Write(raw[:ticketPayloadBytes])
	if !hmac.Equal(raw[ticketPayloadBytes:], mac.Sum(nil)) || raw[0] != Version {
		return submitScope{}, fmt.Errorf("invalid submission ticket")
	}
	generation, _ := base64.RawURLEncoding.DecodeString(s.generation)
	ownerHash, modelHash := ownerDigest(owner), hashFields(model)
	if !hmac.Equal(raw[1:17], generation) || !hmac.Equal(raw[17:49], ownerHash[:]) || !hmac.Equal(raw[49:81], modelHash[:]) {
		return submitScope{}, fmt.Errorf("submission ticket scope mismatch")
	}
	values := [7]int64{}
	for i := 0; i < 7; i++ {
		offset := 97 + i*8
		n := binary.BigEndian.Uint64(raw[offset : offset+8])
		if n > math.MaxInt64 {
			return submitScope{}, fmt.Errorf("submission ticket numeric overflow")
		}
		values[i] = int64(n)
	}
	limits := Limits{
		RequestBytes: values[1], ResultBytes: values[2],
		ExecutionTimeoutMS: values[3], RetentionMS: values[4],
		ControlTimeoutMS: values[5], FinalizationTimeoutMS: values[6],
		ReadBytes: int(binary.BigEndian.Uint32(raw[153:157])),
	}
	if values[0] <= 0 || ValidateLimits(limits) != nil {
		return submitScope{}, fmt.Errorf("invalid submission ticket limits")
	}
	digest := sha256.Sum256([]byte(token))
	return submitScope{untilMS: values[0], limits: limits, digest: hex.EncodeToString(digest[:])}, nil
}

func addMilliseconds(base int64, values ...int64) (int64, error) {
	if base < 0 {
		return 0, fmt.Errorf("invalid deferred timestamp")
	}
	for _, value := range values {
		if value < 0 || value > math.MaxInt64-base {
			return 0, fmt.Errorf("deferred timestamp overflow")
		}
		base += value
	}
	return base, nil
}
