//go:build aix || android || darwin || dragonfly || freebsd || illumos || ios || linux || netbsd || openbsd || solaris

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestForcedOutputParentAliasAfterPreflight(t *testing.T) {
	directory := t.TempDir()
	first, second := filepath.Join(directory, "a"), filepath.Join(directory, "b")
	for _, parent := range []string{first, second} {
		if err := os.Mkdir(parent, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(parent, "result.json"), []byte("original"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	outputs, err := prepareOutputs(filepath.Join(first, "result.json"), filepath.Join(second, "result.json"), true)
	if err != nil {
		t.Fatal(err)
	}
	outputs[0].data, outputs[1].data = []byte("profile"), []byte("metadata")
	moved := filepath.Join(directory, "b-moved")
	if err := os.Rename(second, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(first, second); err != nil {
		t.Fatal(err)
	}
	if err := publishOutputs(outputs, true); err == nil {
		t.Error("changed parent alias was accepted as a successful forced publication")
	}
	for _, parent := range []string{first, moved} {
		data, err := os.ReadFile(filepath.Join(parent, "result.json"))
		if err != nil || !bytes.Equal(data, []byte("original")) {
			t.Error("changed parent identity was not rejected before replacement")
		}
		entries, err := os.ReadDir(parent)
		if err != nil || len(entries) != 1 {
			t.Error("failed parent validation retained temporary outputs")
		}
	}
}

func TestOutputDeclaredParentSymlinkChangedAfterPreflight(t *testing.T) {
	directory := t.TempDir()
	first, second, alias := filepath.Join(directory, "a"), filepath.Join(directory, "b"), filepath.Join(directory, "alias")
	for _, parent := range []string{first, second} {
		if err := os.Mkdir(parent, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(parent, "result.json"), []byte("original"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(second, alias); err != nil {
		t.Fatal(err)
	}
	outputs, err := prepareOutputs(filepath.Join(first, "result.json"), filepath.Join(alias, "result.json"), true)
	if err != nil {
		t.Fatal(err)
	}
	outputs[0].data, outputs[1].data = []byte("profile"), []byte("metadata")
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(first, alias); err != nil {
		t.Fatal(err)
	}
	if err := publishOutputs(outputs, true); err == nil {
		t.Fatal("changed declared parent symlink was ignored")
	}
	for _, parent := range []string{first, second} {
		if data, err := os.ReadFile(filepath.Join(parent, "result.json")); err != nil || string(data) != "original" {
			t.Fatal("changed declared parent was not rejected before replacement")
		}
	}
}

func TestOutputParentDirectoryReplacementAfterPreflight(t *testing.T) {
	directory := t.TempDir()
	first, second := filepath.Join(directory, "a"), filepath.Join(directory, "b")
	for _, parent := range []string{first, second} {
		if err := os.Mkdir(parent, 0700); err != nil {
			t.Fatal(err)
		}
	}
	profilePath := filepath.Join(first, "result.json")
	if err := os.WriteFile(profilePath, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	outputs, err := prepareOutputs(profilePath, filepath.Join(second, "result.json"), true)
	if err != nil {
		t.Fatal(err)
	}
	outputs[0].data, outputs[1].data = []byte("profile"), []byte("metadata")
	if err := os.Rename(second, filepath.Join(directory, "b-moved")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(second, 0700); err != nil {
		t.Fatal(err)
	}
	if err := publishOutputs(outputs, true); err == nil {
		t.Fatal("replacement parent directory was accepted")
	}
	if data, err := os.ReadFile(profilePath); err != nil || string(data) != "original" {
		t.Fatal("replacement parent was not rejected before the first publication")
	}
}

func TestOutputOperationsStayAnchoredAfterParentRename(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(map[bool]string{false: "no-overwrite", true: "force"}[force], func(t *testing.T) {
			directory := t.TempDir()
			first, second, moved := filepath.Join(directory, "a"), filepath.Join(directory, "b"), filepath.Join(directory, "b-moved")
			for _, parent := range []string{first, second} {
				if err := os.Mkdir(parent, 0700); err != nil {
					t.Fatal(err)
				}
			}
			outputs, err := prepareOutputs(filepath.Join(first, "result.json"), filepath.Join(second, "result.json"), force)
			if err != nil {
				t.Fatal(err)
			}
			outputs[0].data, outputs[1].data = []byte("profile"), []byte("metadata")
			anchors := testOutputAnchors(t, outputs, force)
			if err := os.Rename(second, moved); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(first, second); err != nil {
				t.Fatal(err)
			}
			if err := validateOutputParents(anchors); err == nil {
				t.Fatal("parent checks did not detect a post-anchor rename")
			}
			for _, anchor := range anchors {
				staged, err := stageOutput(anchor)
				if err != nil {
					t.Fatal(err)
				}
				if err := publishAnchoredOutput(anchor, staged, force); err != nil {
					t.Fatal(err)
				}
				removeStagedOutput(anchor.root, staged)
			}
			for index, parent := range []string{first, moved} {
				data, err := os.ReadFile(filepath.Join(parent, "result.json"))
				if err != nil || !bytes.Equal(data, outputs[index].data) {
					t.Fatal("anchored publication followed a rebound parent name")
				}
				entries, _ := os.ReadDir(parent)
				if len(entries) != 1 {
					t.Fatal("anchored cleanup followed a rebound parent name")
				}
			}
		})
	}
}

func TestOutputParentRenameBetweenCheckAndReplacement(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(map[bool]string{false: "no-overwrite", true: "force"}[force], func(t *testing.T) {
			directory := t.TempDir()
			first, second, moved := filepath.Join(directory, "a"), filepath.Join(directory, "b"), filepath.Join(directory, "a-moved")
			for _, parent := range []string{first, second} {
				if err := os.Mkdir(parent, 0700); err != nil {
					t.Fatal(err)
				}
				if force {
					if err := os.WriteFile(filepath.Join(parent, "result.json"), []byte("original"), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			outputs, err := prepareOutputs(filepath.Join(first, "result.json"), filepath.Join(second, "result.json"), force)
			if err != nil {
				t.Fatal(err)
			}
			outputs[0].data, outputs[1].data = []byte("profile"), []byte("metadata")
			anchors := testOutputAnchors(t, outputs, force)
			calls := 0
			err = publishOutputAnchors(anchors, force, func(anchor outputAnchor, staged stagedOutput, force bool) error {
				calls++
				if calls == 1 {
					if err := os.Rename(first, moved); err != nil {
						return err
					}
					if err := os.Symlink(second, first); err != nil {
						return err
					}
				}
				return publishAnchoredOutput(anchor, staged, force)
			})
			if err == nil || calls != 1 {
				t.Fatal("parent rename during replacement was hidden")
			}
			if force {
				profile, profileErr := os.ReadFile(filepath.Join(moved, "result.json"))
				metadata, metadataErr := os.ReadFile(filepath.Join(second, "result.json"))
				if profileErr != nil || metadataErr != nil || string(profile) != "profile" || string(metadata) != "original" {
					t.Fatal("forced parent race escaped the originally validated roots")
				}
			} else {
				for _, parent := range []string{moved, second} {
					entries, _ := os.ReadDir(parent)
					if len(entries) != 0 {
						t.Fatal("rollback/cleanup followed a rebound parent path")
					}
				}
			}
		})
	}
}

func TestOutputDestinationTypeChangesBeforeReplacement(t *testing.T) {
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
	anchors := testOutputAnchors(t, outputs, true)
	moved := filepath.Join(directory, "original-profile.json")
	err = publishOutputAnchors(anchors, true, func(anchor outputAnchor, staged stagedOutput, force bool) error {
		if err := os.Rename(profilePath, moved); err != nil {
			return err
		}
		if err := os.Symlink(metadataPath, profilePath); err != nil {
			return err
		}
		return publishAnchoredOutput(anchor, staged, force)
	})
	if err == nil {
		t.Fatal("forced replacement accepted a changed nonregular destination")
	}
	if info, err := os.Lstat(profilePath); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("forced publication replaced an output symlink")
	}
	for _, path := range []string{moved, metadataPath} {
		if data, err := os.ReadFile(path); err != nil || string(data) != "original" {
			t.Fatal("destination type rejection changed another file")
		}
	}
}

func TestSpecFIFOAndOutputSymlinksFailWithoutBlocking(t *testing.T) {
	directory := t.TempDir()
	fifo := filepath.Join(directory, "pipe")
	if err := unix.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, err := readSpec(fifo); err == nil || time.Since(start) > time.Second {
		t.Fatal("FIFO spec blocked or was accepted")
	}
	if _, err := prepareOutputs(fifo, filepath.Join(directory, "metadata"), true); err == nil {
		t.Fatal("FIFO output was accepted")
	}
	original := filepath.Join(directory, "original")
	link := filepath.Join(directory, "link")
	if err := os.WriteFile(original, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(original, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readSpec(link); err != nil {
		t.Fatal("operator-owned symlink to a regular input was rejected")
	}
	if _, err := prepareOutputs(link, filepath.Join(directory, "metadata"), true); err == nil {
		t.Fatal("--force followed an output symlink")
	}
	if data, err := os.ReadFile(original); err != nil || string(data) != "original" {
		t.Fatal("symlink rejection changed the existing target")
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(directory, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareOutputs(filepath.Join(directory, "same"), filepath.Join(alias, "same"), false); err == nil {
		t.Fatal("parent-directory symlink allowed colliding outputs")
	}
}
