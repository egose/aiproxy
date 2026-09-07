package configedit

import (
	"fmt"
	"sync"
	"testing"
)

func TestConcurrentSecretsUpdatesPreserveKeys(t *testing.T) {
	dir := t.TempDir()
	secretsPath := dir + "/keys.json"
	const writers = 16
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%02d", i)
			if err := WriteSecretsUpdate(SecretsUpdate{Path: secretsPath, Key: key, Value: "value-" + key}); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("WriteSecretsUpdate(): %v", err)
	}
	got, err := ReadSecretsFile(secretsPath)
	if err != nil {
		t.Fatalf("ReadSecretsFile(): %v", err)
	}
	if len(got) != writers {
		t.Fatalf("keys = %d, want %d (%v)", len(got), writers, got)
	}
	for i := 0; i < writers; i++ {
		key := fmt.Sprintf("key-%02d", i)
		if got[key] != "value-"+key {
			t.Fatalf("key %q = %q", key, got[key])
		}
	}
}

func TestConcurrentConfigureAndSecretsWriters(t *testing.T) {
	dir := t.TempDir()
	secretsPath := dir + "/keys.json"
	configPath := dir + "/config.hcl"
	if err := WriteSecretsUpdate(SecretsUpdate{Path: secretsPath, Key: "seed", Value: "seed-value"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("flat-%d", i)
			if err := WriteSecretsUpdate(SecretsUpdate{Path: secretsPath, Key: key, Value: "v"}); err != nil {
				errs <- err
			}
		}(i)
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			source := "listener \"http\" \"public\" {\n  address = \":8080\"\n}\n"
			key := fmt.Sprintf("provider-%d", i)
			if err := WriteProviderFiles(configPath, source, SecretsUpdate{Path: secretsPath, Key: key, Value: "v"}); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("writer: %v", err)
	}
	got, err := ReadSecretsFile(secretsPath)
	if err != nil {
		t.Fatalf("ReadSecretsFile(): %v", err)
	}
	for _, key := range []string{"seed", "flat-0", "flat-1", "flat-2", "flat-3", "provider-0", "provider-1", "provider-2", "provider-3"} {
		if _, ok := got[key]; !ok {
			t.Fatalf("lost key %q in %v", key, got)
		}
	}
}
