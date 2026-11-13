package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/tlsprofile"
)

func TestSecureOutputPublishingAndForce(t *testing.T) {
	directory := t.TempDir()
	profilePath, metadataPath := filepath.Join(directory, "profile.json"), filepath.Join(directory, "metadata.json")
	outputs, err := prepareOutputs(profilePath, metadataPath, false)
	if err != nil {
		t.Fatal(err)
	}
	outputs[0].data, outputs[1].data = []byte("profile\n"), []byte("metadata\n")
	if err := publishOutputs(outputs, false); err != nil {
		t.Fatal(err)
	}
	for _, output := range outputs {
		info, err := os.Stat(output.path)
		if err != nil || !info.Mode().IsRegular() || runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
			t.Fatal("published output was not a secure regular file")
		}
		data, _ := os.ReadFile(output.path)
		if !bytes.Equal(data, output.data) {
			t.Fatal("published output was incomplete")
		}
	}
	if err := publishOutputs(outputs, false); err == nil {
		t.Fatal("publishing overwrote existing outputs without --force")
	}
	outputs, err = prepareOutputs(profilePath, metadataPath, true)
	if err != nil {
		t.Fatal(err)
	}
	outputs[0].data, outputs[1].data = []byte("replacement\n"), []byte("replacement metadata\n")
	if err := publishOutputs(outputs, true); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(directory)
	if len(entries) != 2 {
		t.Fatal("temporary staged outputs were retained")
	}
}

func TestOutputCollisionAndInvalidDestinations(t *testing.T) {
	directory := t.TempDir()
	profilePath, metadataPath := filepath.Join(directory, "profile.json"), filepath.Join(directory, "metadata.json")
	for _, paths := range [][2]string{{profilePath, profilePath}, {profilePath, filepath.Join(directory, ".", "profile.json")}, {directory, metadataPath}, {"", metadataPath}, {"private-sentinel\n", metadataPath}, {filepath.Join(directory, "missing", "profile.json"), metadataPath}} {
		if _, err := prepareOutputs(paths[0], paths[1], false); err == nil || strings.Contains(err.Error(), "private-sentinel") {
			t.Fatal("invalid output paths did not fail privately")
		}
	}
	if err := os.WriteFile(profilePath, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(profilePath, metadataPath); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareOutputs(profilePath, metadataPath, true); err == nil || !strings.Contains(err.Error(), "collide") {
		t.Fatal("hardlinked profile and metadata destinations were not rejected")
	}
}

func TestOutputStagingAndPreflightRacePreserveExistingFiles(t *testing.T) {
	directory := t.TempDir()
	profilePath, metadataPath := filepath.Join(directory, "profile.json"), filepath.Join(directory, "metadata.json")
	outputs, err := prepareOutputs(profilePath, metadataPath, false)
	if err != nil {
		t.Fatal(err)
	}
	outputs[0].data, outputs[1].data = []byte("profile"), []byte("metadata")
	if err := os.WriteFile(metadataPath, []byte("foreign output"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := publishOutputs(outputs, false); err == nil {
		t.Fatal("post-preflight output was overwritten")
	}
	data, _ := os.ReadFile(metadataPath)
	if string(data) != "foreign output" {
		t.Fatal("publishing changed an output it did not own")
	}
	if _, err := os.Stat(profilePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed output staging published a partial profile")
	}
	entries, _ := os.ReadDir(directory)
	if len(entries) != 1 {
		t.Fatal("failed publishing retained staged files")
	}
}

func TestOutputRollbackOnlyRemovesOwnedLinks(t *testing.T) {
	directory := t.TempDir()
	staged := filepath.Join(directory, "staged")
	owned := filepath.Join(directory, "owned")
	foreign := filepath.Join(directory, "foreign")
	if err := os.WriteFile(staged, []byte("ours"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(staged, owned); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(foreign, []byte("not ours"), 0600); err != nil {
		t.Fatal(err)
	}
	outputs, err := prepareOutputs(owned, foreign, true)
	if err != nil {
		t.Fatal(err)
	}
	anchors, err := openOutputAnchors(outputs)
	if err != nil {
		t.Fatal(err)
	}
	defer closeOutputAnchors(anchors)
	info, err := os.Lstat(staged)
	if err != nil {
		t.Fatal(err)
	}
	rollbackPublished(anchors, []stagedOutput{{name: "staged", info: info}, {name: "staged", info: info}})
	if _, err := os.Stat(owned); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("owned partial output was not rolled back")
	}
	if data, err := os.ReadFile(foreign); err != nil || string(data) != "not ours" {
		t.Fatal("rollback removed or changed someone else's output")
	}
}

func TestOutputPairCollisionAfterPreflight(t *testing.T) {
	directory := t.TempDir()
	profilePath, metadataPath := filepath.Join(directory, "profile.json"), filepath.Join(directory, "metadata.json")
	for _, path := range []string{profilePath, metadataPath} {
		if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	outputs, err := prepareOutputs(profilePath, metadataPath, true)
	if err != nil {
		t.Fatal(err)
	}
	outputs[0].data, outputs[1].data = []byte("profile"), []byte("metadata")
	if err := os.Remove(metadataPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(profilePath, metadataPath); err != nil {
		t.Fatal(err)
	}
	if err := publishOutputs(outputs, true); err == nil || !strings.Contains(err.Error(), "collide") {
		t.Fatal("publication did not revalidate anchored destination identities")
	}
	for _, path := range []string{profilePath, metadataPath} {
		if data, err := os.ReadFile(path); err != nil || string(data) != "original" {
			t.Fatal("destination collision was not rejected before replacement")
		}
	}
}

func TestSecondOutputPublicationFailurePreservesOverwritePolicy(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(map[bool]string{false: "no-overwrite", true: "force"}[force], func(t *testing.T) {
			directory := t.TempDir()
			profilePath, metadataPath := filepath.Join(directory, "profile.json"), filepath.Join(directory, "metadata.json")
			if force {
				for _, path := range []string{profilePath, metadataPath} {
					if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			outputs, err := prepareOutputs(profilePath, metadataPath, force)
			if err != nil {
				t.Fatal(err)
			}
			outputs[0].data, outputs[1].data = []byte("profile"), []byte("metadata")
			anchors := testOutputAnchors(t, outputs, force)
			calls := 0
			err = publishOutputAnchors(anchors, force, func(anchor outputAnchor, staged stagedOutput, force bool) error {
				calls++
				if calls == 2 {
					anchor.name = filepath.Join("missing-parent", anchor.name)
				}
				return publishAnchoredOutput(anchor, staged, force)
			})
			if err == nil || calls != 2 {
				t.Fatal("second filesystem publication failure was hidden")
			}
			if force {
				profile, profileErr := os.ReadFile(profilePath)
				metadata, metadataErr := os.ReadFile(metadataPath)
				if profileErr != nil || metadataErr != nil || string(profile) != "profile" || string(metadata) != "original" {
					t.Fatal("forced failure did not preserve its documented partial-publication outcome")
				}
			} else {
				for _, path := range []string{profilePath, metadataPath} {
					if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
						t.Fatal("non-forced failure did not roll back only its published links")
					}
				}
			}
			entries, _ := os.ReadDir(directory)
			want := 0
			if force {
				want = 2
			}
			if len(entries) != want {
				t.Fatal("failed second publication retained staged files")
			}
		})
	}
}

func testOutputAnchors(t *testing.T, outputs []outputFile, force bool) []outputAnchor {
	t.Helper()
	anchors, err := openOutputAnchors(outputs)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeOutputAnchors(anchors) })
	if err := validateOutputAnchors(anchors, force); err != nil {
		t.Fatal(err)
	}
	return anchors
}

func TestSpecReadBoundsAndRegularFiles(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "profile.json")
	for _, length := range []int{tlsprofile.MaxSpecBytes, tlsprofile.MaxSpecBytes + 1} {
		if err := os.WriteFile(path, make([]byte, length), 0600); err != nil {
			t.Fatal(err)
		}
		data, err := readSpec(path)
		if length == tlsprofile.MaxSpecBytes && (err != nil || len(data) != length) {
			t.Fatal("exact read bound failed")
		}
		if length > tlsprofile.MaxSpecBytes && (err == nil || data != nil) {
			t.Fatal("oversized spec was read without a bound")
		}
	}
	for _, path := range []string{directory, filepath.Join(directory, "missing-private-sentinel"), "private-sentinel\n"} {
		if _, err := readSpec(path); err == nil || strings.Contains(err.Error(), "private-sentinel") {
			t.Fatal("nonregular/missing spec was accepted or leaked its path")
		}
	}
}
