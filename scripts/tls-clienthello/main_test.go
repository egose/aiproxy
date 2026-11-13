package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/tlsprofile"
)

func TestCommandOptionsAndUsage(t *testing.T) {
	for _, command := range []string{"capture", "compare"} {
		arguments := []string{"--bun", "bun", "--api", "fetch", "--samples", "3"}
		if command == "capture" {
			arguments = append(arguments, "--out", "tls/profile.json", "--metadata", "tls/metadata.json")
		} else {
			arguments = append(arguments, "--spec", "tls/profile.json")
		}
		if options, err := parseCommandOptions(command, arguments); err != nil || options.samples != 3 || options.api != "fetch" || options.bun != "bun" {
			t.Fatalf("documented spaced arguments failed: %v", err)
		}
		if _, err := parseCommandOptions(command, []string{"--help"}); !errors.Is(err, flag.ErrHelp) {
			t.Fatal("command help was rejected")
		}
	}
	tests := [][]string{
		{}, {"--bun", ""}, {"--api", "node:http"}, {"--samples", "1"}, {"--samples", "17"},
		{"--samples", "three"}, {"--out", "one"}, {"--out", "one", "--metadata", "two", "extra"},
		{"--experimental-http2"}, {"--private-sentinel"},
	}
	for _, arguments := range tests {
		if _, err := parseCommandOptions("capture", arguments); err == nil || strings.Contains(err.Error(), "private-sentinel") {
			t.Fatalf("invalid options accepted or exposed: %v", err)
		}
	}
	if _, err := parseCommandOptions("compare", nil); err == nil {
		t.Fatal("compare did not require a spec")
	}
	for _, arguments := range [][]string{nil, {"help"}, {"--help"}, {"capture", "--help"}, {"compare", "--help"}} {
		var stdout bytes.Buffer
		if err := run(context.Background(), arguments, &stdout); err != nil || !strings.Contains(stdout.String(), "--samples 3") {
			t.Fatal("usage did not include executable spaced commands")
		}
	}
	if err := run(context.Background(), []string{"private-sentinel"}, io.Discard); err == nil || strings.Contains(err.Error(), "private-sentinel") {
		t.Fatal("unknown command was not rejected privately")
	}
}

func TestCaptureCompareCommandRoundTripAndOverwriteProtection(t *testing.T) {
	executable := fixtureExecutable(t, "success")
	directory := t.TempDir()
	profilePath, metadataPath := filepath.Join(directory, "profile.json"), filepath.Join(directory, "metadata.json")
	arguments := []string{"capture", "--bun", executable, "--api", "fetch", "--samples", "3", "--out", profilePath, "--metadata", metadataPath}
	var stdout bytes.Buffer
	if err := run(context.Background(), arguments, &stdout); err != nil || !strings.Contains(stdout.String(), "3 fresh uTLS samples matched") {
		t.Fatalf("capture command failed: %v", err)
	}
	profileData, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := tlsprofile.ParseCustom(profileData)
	if err != nil || profile.Identity() != toolFixtureProfile(t).Identity() {
		t.Fatal("capture did not publish a loadable measured profile")
	}
	metadataData, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	var metadata captureMetadata
	if err := json.Unmarshal(metadataData, &metadata); err != nil || !metadata.Comparison.Matched || metadata.SampleCount != 3 || metadata.Comparison.UTLSSamples != 3 ||
		metadata.Bun.Version != "1.4.4" || metadata.Dependency != tlsprofile.DependencyVersion || metadata.SpecSchema != tlsprofile.SchemaVersion || metadata.Policy != tlsprofile.PolicyVersion ||
		metadata.Conditions.Proxy || metadata.Conditions.RequestBody || metadata.Conditions.TLSVerificationBypass || metadata.Conditions.InheritedEnvironment {
		t.Fatalf("capture metadata was incomplete or incorrect: %v", err)
	}
	if bytes.Contains(metadataData, []byte(directory)) || bytes.Contains(metadataData, []byte(executable)) || bytes.Contains(metadataData, []byte("private-sentinel")) {
		t.Fatal("metadata included machine paths or credentials")
	}
	if invocationCount(t, executable) != 3 {
		t.Fatal("capture did not start exactly three fresh Bun processes")
	}
	stdout.Reset()
	if err := run(context.Background(), arguments, &stdout); err == nil || !strings.Contains(err.Error(), "--force") || invocationCount(t, executable) != 3 {
		t.Fatal("existing files were overwritten or capture ran before overwrite validation")
	}
	unchanged, _ := os.ReadFile(profilePath)
	if !bytes.Equal(unchanged, profileData) {
		t.Fatal("overwrite refusal changed the existing profile")
	}
	stdout.Reset()
	if err := run(context.Background(), []string{"compare", "--bun", executable, "--api", "fetch", "--samples", "3", "--spec", profilePath}, &stdout); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(stdout.Bytes(), &metadata); err != nil || !metadata.Comparison.Matched || invocationCount(t, executable) != 6 {
		t.Fatal("compare did not report fresh structural matching")
	}
	if err := os.WriteFile(profilePath, []byte("operator-owned old output"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run(context.Background(), append(arguments, "--force"), io.Discard); err != nil {
		t.Fatal(err)
	}
	replaced, _ := os.ReadFile(profilePath)
	if !bytes.Equal(replaced, profileData) || invocationCount(t, executable) != 9 {
		t.Fatal("explicit --force did not replace the regular output")
	}
}

func TestCaptureComparisonFailurePublishesNothing(t *testing.T) {
	executable := fixtureExecutable(t, "record-mismatch")
	directory := t.TempDir()
	profilePath, metadataPath := filepath.Join(directory, "profile.json"), filepath.Join(directory, "metadata.json")
	err := run(context.Background(), []string{"capture", "--bun", executable, "--samples", "2", "--out", profilePath, "--metadata", metadataPath}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "comparison failed") {
		t.Fatalf("record mismatch did not fail capture: %v", err)
	}
	for _, path := range []string{profilePath, metadataPath} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("failed comparison published a misleading profile or metadata")
		}
	}
}

func TestCaptureRuntimeDriftPublishesNothing(t *testing.T) {
	executable := fixtureExecutable(t, "runtime-changes")
	directory := t.TempDir()
	err := run(context.Background(), []string{"capture", "--bun", executable, "--samples", "3", "--out", filepath.Join(directory, "profile.json"), "--metadata", filepath.Join(directory, "metadata.json")}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "runtime metadata changed") {
		t.Fatalf("changing runtime identity was not rejected: %v", err)
	}
	entries, _ := os.ReadDir(directory)
	if len(entries) != 0 {
		t.Fatal("runtime drift published a misleading artifact")
	}
}

func TestMeasuredExamplesMatchTheSanitizedFixture(t *testing.T) {
	profileData, err := os.ReadFile("../../examples/tls/bun-1.4.4-fetch.json")
	if err != nil {
		t.Fatal(err)
	}
	profile, err := tlsprofile.ParseCustom(profileData)
	if err != nil || profile.Identity() != toolFixtureProfile(t).Identity() {
		t.Fatal("measured example no longer matches the parser/export fixture")
	}
	metadataData, err := os.ReadFile("../../examples/tls/bun-1.4.4-fetch.metadata.json")
	if err != nil {
		t.Fatal(err)
	}
	var metadata captureMetadata
	if json.Unmarshal(metadataData, &metadata) != nil || metadata.Bun.Version != "1.4.4" || metadata.Bun.Platform != "linux" || metadata.Bun.Arch != "x64" ||
		metadata.SampleCount != 3 || !metadata.Comparison.Matched || metadata.Comparison.BunSamples != 3 || metadata.Comparison.UTLSSamples != 3 ||
		metadata.Comparison.Structure.RecordBytes != 1500 || metadata.Comparison.NormalizedSHA256 != "a766b8a372feecaec084632ceffb7c99bd3398e21547ec347246da6b8dc7327e" {
		t.Fatal("measured example metadata drifted from its actual capture evidence")
	}
	expected := newCaptureMetadata(metadata.Bun, profile, 3, metadata.Comparison)
	if metadata.SpecSHA256 != expected.SpecSHA256 || metadata.ProfileID != expected.ProfileID || metadata.Dependency != expected.Dependency || metadata.SpecSchema != expected.SpecSchema || metadata.Policy != expected.Policy {
		t.Fatal("measured example pins/digest do not describe its loadable recipe")
	}
	for _, data := range [][]byte{profileData, metadataData} {
		for _, forbidden := range []string{"/home/", "/tmp/", "executable", "hostname", `"key_exchange"`, `"session_id"`, "api_key", "private-sentinel"} {
			if bytes.Contains(data, []byte(forbidden)) {
				t.Fatal("measured shared artifact includes private capture or machine data")
			}
		}
	}
}

func TestCompareMismatchIsNonzeroAndStructured(t *testing.T) {
	executable := fixtureExecutable(t, "success")
	profilePath := filepath.Join(t.TempDir(), "changed.json")
	data := bytes.Replace(toolFixtureProfile(t).CanonicalJSON(), []byte(`"protocol_name_list":["http/1.1"]`), []byte(`"protocol_name_list":["h2","http/1.1"]`), 1)
	if _, err := tlsprofile.ParseCustom(data); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(profilePath, data, 0600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	err := run(context.Background(), []string{"compare", "--bun", executable, "--samples", "2", "--spec", profilePath}, &stdout)
	var metadata captureMetadata
	if err == nil || json.Unmarshal(stdout.Bytes(), &metadata) != nil || metadata.Comparison.Matched || len(metadata.Comparison.Differences) != 2 {
		t.Fatalf("compare mismatch was hidden: %v", err)
	}
}

func TestCommandValidatesPathsAndSpecsBeforeStartingBun(t *testing.T) {
	executable := fixtureExecutable(t, "success")
	directory := t.TempDir()
	for _, arguments := range [][]string{
		{"capture", "--bun", executable, "--out", filepath.Join(directory, "same"), "--metadata", filepath.Join(directory, "same")},
		{"compare", "--bun", executable, "--spec", filepath.Join(directory, "missing")},
	} {
		if err := run(context.Background(), arguments, io.Discard); err == nil || invocationCount(t, executable) != 0 {
			t.Fatal("invalid paths reached the Bun subprocess")
		}
	}
	invalid := filepath.Join(directory, "invalid.json")
	if err := os.WriteFile(invalid, []byte(`{"private-sentinel":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run(context.Background(), []string{"compare", "--bun", executable, "--spec", invalid}, io.Discard); err == nil || strings.Contains(err.Error(), "private-sentinel") || invocationCount(t, executable) != 0 {
		t.Fatal("invalid profile was accepted, exposed or reached Bun")
	}
}
