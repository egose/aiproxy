package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/config"
)

func TestConvertSecureReplacement(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "input.hcl")
	dst := filepath.Join(dir, "output.json")
	t.Setenv("SAFE02_TEST_SECRET", "synthetic-conversion-secret")
	source := strings.Replace(convertTestSource, `"sk-test"`, `env("SAFE02_TEST_SECRET")`, 1)
	if err := os.WriteFile(src, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dst, 0o644); err != nil {
		t.Fatal(err)
	}
	old, err := os.Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	out, err := runConvertCommand(t, nil, dst, "--config", src, "--force")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	want, _, _, err := config.Convert([]byte(source), src, false)
	if err != nil {
		t.Fatal(err)
	}
	assertConvertedFile(t, dst, want)
	if !bytes.Contains(want, []byte("synthetic-conversion-secret")) || strings.Contains(out, "synthetic-conversion-secret") {
		t.Fatal("secret must be materialized only in the converted file")
	}
	raw, err := io.ReadAll(old)
	if err != nil || string(raw) != "old" {
		t.Fatalf("old open file changed: %q, %v", raw, err)
	}
	assertConvertTempsRemoved(t, dst)
}

func TestConvertRejectsUnsafeDestination(t *testing.T) {
	for _, kind := range []string{"live-link", "dangling-link", "directory"} {
		for _, force := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/force=%t", kind, force), func(t *testing.T) {
				dir := t.TempDir()
				src := filepath.Join(dir, "input.hcl")
				dst := filepath.Join(dir, "output.json")
				referent := filepath.Join(dir, "referent")
				if err := os.WriteFile(src, []byte(convertTestSource), 0o600); err != nil {
					t.Fatal(err)
				}
				if kind == "directory" {
					if err := os.Mkdir(dst, 0o700); err != nil {
						t.Fatal(err)
					}
				} else {
					if kind == "live-link" {
						if err := os.WriteFile(referent, []byte("sentinel"), 0o644); err != nil {
							t.Fatal(err)
						}
					}
					if err := os.Symlink(referent, dst); err != nil {
						t.Fatal(err)
					}
				}
				args := []string{dst, "--config", src}
				if force {
					args = append(args, "--force")
				}
				out, err := runConvertCommand(t, nil, args...)
				if err == nil || strings.Contains(out, "converted ") {
					t.Fatalf("unsafe destination accepted: %v, %q", err, out)
				}
				info, err := os.Lstat(dst)
				if err != nil {
					t.Fatal(err)
				}
				if kind == "directory" {
					if !info.IsDir() {
						t.Fatal("directory replaced")
					}
				} else {
					if info.Mode()&os.ModeSymlink == 0 {
						t.Fatal("symlink replaced")
					}
					raw, err := os.ReadFile(referent)
					if kind == "live-link" && (err != nil || string(raw) != "sentinel") {
						t.Fatalf("referent changed: %q, %v", raw, err)
					}
					if kind == "dangling-link" && !errors.Is(err, os.ErrNotExist) {
						t.Fatalf("dangling referent created: %v", err)
					}
				}
				assertConvertTempsRemoved(t, dst)
			})
		}
	}
}

func TestConvertConcurrentNonForce(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "output.json")
	const publishers = 16
	type result struct {
		want []byte
		err  error
	}
	results := make(chan result, publishers)
	start := make(chan struct{})
	for i := 0; i < publishers; i++ {
		src := filepath.Join(dir, fmt.Sprintf("input-%d.hcl", i))
		source := strings.Replace(convertTestSource, "sk-test", fmt.Sprintf("synthetic-%d", i), 1)
		if err := os.WriteFile(src, []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
		want, _, _, err := config.Convert([]byte(source), src, false)
		if err != nil {
			t.Fatal(err)
		}
		cmd := newConvertCommand()
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		cmd.SetArgs([]string{dst, "--config", src})
		go func() {
			<-start
			results <- result{want, cmd.Execute()}
		}()
	}
	close(start)
	winners := 0
	var winner []byte
	for i := 0; i < publishers; i++ {
		r := <-results
		if r.err == nil {
			winners++
			winner = r.want
		} else if !strings.Contains(r.err.Error(), "--force") {
			t.Errorf("unexpected loser error: %v", r.err)
		}
	}
	if winners != 1 {
		t.Fatalf("successful publishers = %d, want 1", winners)
	}
	assertConvertedFile(t, dst, winner)
	assertConvertTempsRemoved(t, dst)
}

func TestConvertValidationBeforePublication(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(fmt.Sprintf("force=%t", force), func(t *testing.T) {
			dir := t.TempDir()
			src := filepath.Join(dir, "invalid.hcl")
			dst := filepath.Join(dir, "output.json")
			if err := os.WriteFile(src, []byte(strings.Replace(convertTestSource, "sk-test", "", 1)), 0o600); err != nil {
				t.Fatal(err)
			}
			args := []string{dst, "--config", src}
			if force {
				if err := os.WriteFile(dst, []byte("sentinel"), 0o644); err != nil {
					t.Fatal(err)
				}
				args = append(args, "--force")
			}
			if _, err := runConvertCommand(t, nil, args...); err == nil {
				t.Fatal("invalid config converted")
			}
			raw, err := os.ReadFile(dst)
			if force && (err != nil || string(raw) != "sentinel") {
				t.Fatalf("old file changed: %q, %v", raw, err)
			}
			if !force && !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("invalid file published: %v", err)
			}
			out, err := runConvertCommand(t, nil, "-", "--config", src)
			if err == nil || strings.HasPrefix(out, "{") {
				t.Fatalf("invalid config emitted: %v, %q", err, out)
			}
			assertConvertTempsRemoved(t, dst)
		})
	}
}

type convertFailingWriter struct{}

func (convertFailingWriter) Write([]byte) (int, error) {
	return 0, io.ErrClosedPipe
}

func TestConvertOutputFailure(t *testing.T) {
	for _, target := range []string{"-", "output.json"} {
		t.Run(target, func(t *testing.T) {
			dir := t.TempDir()
			src := filepath.Join(dir, "input.hcl")
			if err := os.WriteFile(src, []byte(convertTestSource), 0o600); err != nil {
				t.Fatal(err)
			}
			dst := target
			if target != "-" {
				dst = filepath.Join(dir, target)
			}
			cmd := newConvertCommand()
			cmd.SetOut(convertFailingWriter{})
			cmd.SetErr(io.Discard)
			cmd.SetArgs([]string{dst, "--config", src})
			err := cmd.Execute()
			if !errors.Is(err, io.ErrClosedPipe) {
				t.Fatalf("error = %v, want output failure", err)
			}
			if target != "-" {
				if !strings.Contains(err.Error(), "published") {
					t.Fatalf("error does not distinguish successful publication: %v", err)
				}
				want, _, _, err := config.Convert([]byte(convertTestSource), src, false)
				if err != nil {
					t.Fatal(err)
				}
				assertConvertedFile(t, dst, want)
			}
		})
	}
}

func TestConvertUnsupportedPlatform(t *testing.T) {
	t.Setenv("AIPROXY_CONFIG", convertTestSource)
	for _, force := range []bool{false, true} {
		for _, existing := range []bool{false, true} {
			t.Run(fmt.Sprintf("force=%t/existing=%t", force, existing), func(t *testing.T) {
				dir := t.TempDir()
				parent := filepath.Join(dir, "new")
				target := filepath.Join(parent, "out.json")
				if existing {
					target = filepath.Join(dir, "old.json")
					if err := os.WriteFile(target, []byte("sentinel"), 0o600); err != nil {
						t.Fatal(err)
					}
				}
				cmd := newConvertCommand()
				var out bytes.Buffer
				cmd.SetOut(&out)
				err := runConvertForPlatform(cmd, "", false, force, []string{target}, "windows")
				if err == nil || !strings.Contains(err.Error(), "unsupported on windows") || out.Len() != 0 {
					t.Fatalf("err=%v output=%q", err, &out)
				}
				if existing {
					raw, err := os.ReadFile(target)
					if err != nil || string(raw) != "sentinel" {
						t.Fatalf("old file=%q err=%v", raw, err)
					}
				} else if _, err := os.Stat(parent); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("parent created: %v", err)
				}
				if err := runConvertForPlatform(cmd, "", false, force, []string{"-"}, "windows"); err != nil {
					t.Fatal(err)
				}
				want, _, _, err := config.Convert([]byte(convertTestSource), config.EnvConfigFilename, false)
				if err != nil || !bytes.Equal(out.Bytes(), want) {
					t.Fatalf("stdout differs: %v", err)
				}
			})
		}
	}
}

func assertConvertedFile(t *testing.T, path string, want []byte) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(raw, want) {
		t.Fatalf("converted file differs: %v", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode() != 0o600 {
		t.Fatalf("mode = %v, want regular 0600", info.Mode())
	}
}

func assertConvertTempsRemoved(t *testing.T, path string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("temporary files remain: %v, %v", matches, err)
	}
}
