//go:build unix

package config

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestTLSProfileFileRejectsFIFOAndDevicesWithoutBlocking(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "profile-fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "fifo-link")
	if err := os.Symlink(fifo, link); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{fifo, link, os.DevNull, dir} {
		start := time.Now()
		if _, err := loadTLSProfile(path, ""); err == nil || !strings.Contains(err.Error(), "regular file") {
			t.Fatalf("nonregular source was not rejected: %v", err)
		}
		if time.Since(start) > time.Second {
			t.Fatal("nonregular file inspection blocked")
		}
	}
	type result struct {
		file *os.File
		err  error
	}
	opened := make(chan result, 1)
	go func() {
		file, err := openTLSProfileFile(fifo)
		opened <- result{file, err}
	}()
	select {
	case result := <-opened:
		if result.err != nil {
			t.Fatal(result.err)
		}
		defer result.file.Close()
		info, err := result.file.Stat()
		if err != nil || info.Mode().IsRegular() {
			t.Fatal("opened FIFO cannot be mistaken for a regular file")
		}
	case <-time.After(time.Second):
		t.Fatal("nonblocking open cannot safely handle a stat/open FIFO replacement")
	}
	p := validDynamicProvider()
	p.TLSClientHello = "custom"
	p.TLSClientHelloSpecFile = fifo
	if err := ValidateDynamicProvider(p); err == nil || !strings.Contains(err.Error(), "built-in profiles only") {
		t.Fatalf("managed source crossed the file-access boundary: %v", err)
	}
}

func TestTLSProfileFileStatOpenRaceRemainsNonblocking(t *testing.T) {
	dir := t.TempDir()
	regular := filepath.Join(dir, "regular.json")
	tlsTestFile(t, regular, tlsTestFixture(t))
	fifo := filepath.Join(dir, "profile-fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	path, next := filepath.Join(dir, "profile-link"), filepath.Join(dir, "next-link")
	if err := os.Symlink(regular, path); err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	var writer sync.WaitGroup
	writer.Go(func() {
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			target := regular
			if i%2 != 0 {
				target = fifo
			}
			if err := os.Symlink(target, next); err != nil {
				t.Error(err)
				return
			}
			if err := os.Rename(next, path); err != nil {
				t.Error(err)
				return
			}
		}
	})
	defer func() { close(stop); writer.Wait() }()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			_, _ = loadTLSProfile(path, "")
		}
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("stat/open race blocked on a FIFO")
	}
}
