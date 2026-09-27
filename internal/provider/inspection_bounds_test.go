package provider

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func inspectionAdvertisedFrame(size uint64) []byte {
	raw := []byte{0x28, 0xb5, 0x2f, 0xfd, 0xc0, 0x50}
	raw = binary.LittleEndian.AppendUint64(raw, size)
	return append(raw, 1, 0, 0)
}

func inspectionZstdKnown(tb testing.TB, plain []byte) []byte {
	tb.Helper()
	w, err := zstd.NewWriter(nil, zstd.WithEncoderConcurrency(1), zstd.WithWindowSize(1<<20), zstd.WithSingleSegment(false))
	if err != nil {
		tb.Fatal(err)
	}
	defer w.Close()
	return w.EncodeAll(plain, nil)
}

func TestInspectionBoundariesAndCorruption(t *testing.T) {
	for _, encoding := range []string{"gzip", "deflate", "br", "zstd"} {
		for _, size := range []int{(1 << 20) - 1, 1 << 20, (1 << 20) + 1} {
			t.Run(fmt.Sprintf("%s/%d", encoding, size), func(t *testing.T) {
				plain := bytes.Repeat([]byte("a"), size)
				var raw []byte
				if encoding == "zstd" {
					raw = inspectionZstdKnown(t, plain)
				} else {
					raw = encodeForInspectionTest(t, encoding, plain)
				}
				original := bytes.Clone(raw)
				got, ok := DecodeBodyForInspection(http.Header{"Content-Encoding": {encoding}}, raw)
				if size <= 1<<20 {
					if !ok || !bytes.Equal(got, plain) {
						t.Fatalf("boundary decode: ok=%v len=%d", ok, len(got))
					}
				} else if ok || !bytes.Equal(got, raw) || &got[0] != &raw[0] {
					t.Fatal("oversized decode must return original allocation and failure")
				}
				if !bytes.Equal(raw, original) {
					t.Fatal("input mutated")
				}
				if size == 1<<20 {
					truncated := raw[:len(raw)-1]
					got, ok = DecodeBodyForInspection(http.Header{"Content-Encoding": {encoding}}, truncated)
					if ok || !bytes.Equal(got, truncated) {
						t.Fatal("corrupt boundary stream must be rejected, including its trailer")
					}
				}
			})
		}
	}
}

func TestInspectionZstdFrames(t *testing.T) {
	plain := bytes.Repeat([]byte("a"), 1<<19)
	frame := inspectionZstdKnown(t, plain)
	checksum := bytes.Clone(frame)
	checksum[len(checksum)-1] ^= 0xff
	largeWindow := []byte{0x28, 0xb5, 0x2f, 0xfd, 0, 0x80, 1, 0, 0}
	var hdr zstd.Header
	if err := hdr.Decode(largeWindow); err != nil || hdr.WindowSize != 64<<20 {
		t.Fatalf("invalid large-window fixture: %+v %v", hdr, err)
	}
	for _, tc := range []struct {
		name string
		raw  []byte
		want []byte
	}{
		{"advertised_1TiB", inspectionAdvertisedFrame(1 << 40), nil},
		{"window_64MiB", largeWindow, nil},
		{"unknown_64MiB", inspectionZstdStream(t, 64<<20), nil},
		{"checksum", checksum, nil},
		{"trailing_corruption", append(bytes.Clone(frame), 0xff), nil},
		{"concatenated_boundary", append(bytes.Clone(frame), frame...), bytes.Repeat([]byte("a"), 1<<20)},
		{"concatenated_overflow", append(append(bytes.Clone(frame), frame...), frame...), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := DecodeBodyForInspection(http.Header{"Content-Encoding": {"zstd"}}, tc.raw)
			if tc.want != nil {
				if !ok || !bytes.Equal(got, tc.want) {
					t.Fatalf("valid frames: ok=%v len=%d", ok, len(got))
				}
			} else if ok || !bytes.Equal(got, tc.raw) || &got[0] != &tc.raw[0] {
				t.Fatal("rejected frames must retain original bytes and allocation")
			}
		})
	}
}

func TestInspectionEncodingChains(t *testing.T) {
	plain := []byte(`{"error":"not issued to this caller"}`)
	inner := inspectionZstdKnown(t, plain)
	two := encodeForInspectionTest(t, "gzip", inner)
	four := encodeForInspectionTest(t, "gzip", inspectionZstdKnown(t, two))
	oversize := inspectionZstdStream(t, 2<<20)
	for _, tc := range []struct {
		name   string
		values []string
		raw    []byte
		ok     bool
	}{
		{"two", []string{"zstd, gzip"}, two, true},
		{"split_headers", []string{"zstd", "gzip"}, two, true},
		{"four", []string{"ZSTD, gzip, zstd, GZIP"}, four, true},
		{"five", []string{"identity,zstd,gzip,zstd,gzip"}, four, false},
		{"five_headers", []string{"identity", "zstd", "gzip", "zstd", "gzip"}, four, false},
		{"header_boundary", []string{strings.Repeat(" ", 252) + "zstd"}, inner, true},
		{"header_overflow", []string{strings.Repeat(" ", 253) + "zstd"}, inner, false},
		{"unknown_inner", []string{"compress,gzip"}, two, false},
		{"corrupt_inner", []string{"zstd,gzip"}, encodeForInspectionTest(t, "gzip", []byte("bad")), false},
		{"oversize_inner", []string{"zstd,gzip"}, encodeForInspectionTest(t, "gzip", oversize), false},
		{"oversize_outer", []string{"gzip,zstd"}, oversize, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := DecodeBodyForInspection(http.Header{"Content-Encoding": tc.values}, tc.raw)
			want := tc.raw
			if tc.ok {
				want = plain
			}
			if ok != tc.ok || !bytes.Equal(got, want) {
				t.Fatalf("chain: ok=%v want=%v len=%d", ok, tc.ok, len(got))
			}
		})
	}
}

func inspectionZstdStream(tb testing.TB, size int) []byte {
	tb.Helper()
	var out bytes.Buffer
	w, err := zstd.NewWriter(&out, zstd.WithEncoderConcurrency(1), zstd.WithWindowSize(1<<20))
	if err != nil {
		tb.Fatal(err)
	}
	chunk := bytes.Repeat([]byte("a"), 64<<10)
	for remaining := size; remaining > 0; {
		n := min(remaining, len(chunk))
		if _, err := w.Write(chunk[:n]); err != nil {
			tb.Fatal(err)
		}
		remaining -= n
	}
	if err := w.Close(); err != nil {
		tb.Fatal(err)
	}
	var header zstd.Header
	if err := header.Decode(out.Bytes()); err != nil {
		tb.Fatal(err)
	}
	if header.HasFCS || header.WindowSize > 1<<20 {
		tb.Fatalf("fixture must exercise unknown-size decoding with a bounded window: %+v", header)
	}
	return out.Bytes()
}

func TestInspectionZstdAllocationBound(t *testing.T) {
	for _, size := range []int{2 << 20, 16 << 20, 64 << 20} {
		t.Run(fmt.Sprintf("unknown_%dMiB", size>>20), func(t *testing.T) {
			raw := inspectionZstdStream(t, size)
			header := http.Header{"Content-Encoding": {"zstd"}}
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			for range 4 {
				DecodeBodyForInspection(header, raw)
			}
			runtime.ReadMemStats(&after)
			allocated := (after.TotalAlloc - before.TotalAlloc) / 4
			t.Logf("expanded=%d compressed=%d allocated=%d B/op", size, len(raw), allocated)
			if allocated > 4<<20 {
				t.Fatalf("inspection allocated %d B/op, budget is 4 MiB independent of expansion", allocated)
			}
		})
	}
	for _, tc := range []struct {
		name string
		raw  []byte
	}{
		{"advertised_1TiB", inspectionAdvertisedFrame(1 << 40)},
		{"window_64MiB", []byte{0x28, 0xb5, 0x2f, 0xfd, 0, 0x80, 1, 0, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			for range 4 {
				DecodeBodyForInspection(http.Header{"Content-Encoding": {"zstd"}}, tc.raw)
			}
			runtime.ReadMemStats(&after)
			allocated := (after.TotalAlloc - before.TotalAlloc) / 4
			t.Logf("allocated=%d B/op", allocated)
			if allocated > 2<<20 {
				t.Fatalf("advertised size/window caused %d B/op allocation", allocated)
			}
		})
	}
}

func BenchmarkInspectionZstdBounded(b *testing.B) {
	for _, size := range []int{2 << 20, 16 << 20, 64 << 20} {
		raw := inspectionZstdStream(b, size)
		header := http.Header{"Content-Encoding": {"zstd"}}
		b.Run(fmt.Sprintf("unknown_%dMiB", size>>20), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				DecodeBodyForInspection(header, raw)
			}
		})
	}
	for _, tc := range []struct {
		name string
		raw  []byte
	}{
		{"small_valid", inspectionZstdKnown(b, []byte(`{"error":"not issued to this caller"}`))},
		{"advertised_1TiB", inspectionAdvertisedFrame(1 << 40)},
		{"window_64MiB", []byte{0x28, 0xb5, 0x2f, 0xfd, 0, 0x80, 1, 0, 0}},
	} {
		header := http.Header{"Content-Encoding": {"zstd"}}
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				DecodeBodyForInspection(header, tc.raw)
			}
		})
	}
}
