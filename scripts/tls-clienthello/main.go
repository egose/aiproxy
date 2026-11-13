package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/egose/aiproxy/internal/tlsprofile"
)

const usage = `Local, first-flight Bun fetch ClientHello capture and comparison.

go run ./scripts/tls-clienthello capture --bun bun --api fetch --samples 3 --out tls/bun-fetch.json --metadata tls/bun-fetch.metadata.json
go run ./scripts/tls-clienthello compare --bun bun --api fetch --samples 3 --spec tls/bun-fetch.json

The output directories must exist. Use --force with capture to replace regular
files explicitly. Bun must already be installed. See scripts/tls-clienthello/README.md.
`

type commandOptions struct {
	bun      string
	api      string
	samples  int
	out      string
	metadata string
	spec     string
	force    bool
}

type comparisonResult struct {
	Matched          bool                          `json:"matched"`
	BunSamples       int                           `json:"bun_samples"`
	UTLSSamples      int                           `json:"utls_samples"`
	NormalizedSHA256 string                        `json:"normalized_sha256"`
	Normalization    []string                      `json:"normalization"`
	Structure        tlsprofile.ClientHelloSummary `json:"structure"`
	Differences      []string                      `json:"differences"`
}

type captureConditions struct {
	API                    string   `json:"api"`
	FreshProcess           bool     `json:"fresh_process"`
	URL                    string   `json:"url"`
	ServerName             string   `json:"tls_server_name"`
	Proxy                  bool     `json:"proxy"`
	Method                 string   `json:"method"`
	RequestBody            bool     `json:"request_body"`
	Redirect               string   `json:"redirect"`
	AbortMilliseconds      int      `json:"abort_milliseconds"`
	EmptyBunConfig         bool     `json:"empty_bun_config"`
	Dotenv                 bool     `json:"dotenv"`
	AutoInstall            bool     `json:"auto_install"`
	ExperimentalFlags      []string `json:"experimental_flags"`
	IsolatedHomeAndCWD     bool     `json:"isolated_home_and_cwd"`
	InheritedEnvironment   bool     `json:"inherited_environment"`
	TLSVerificationBypass  bool     `json:"tls_verification_bypass"`
	StopAfterClientHello   bool     `json:"stop_after_client_hello"`
	AcceptReadMilliseconds int      `json:"accept_read_milliseconds"`
	ChildMilliseconds      int      `json:"child_milliseconds"`
	ChildOutputBytes       int      `json:"child_output_bytes_per_stream"`
	MaxRecords             int      `json:"max_records"`
	MaxRecordPayloadBytes  int      `json:"max_record_payload_bytes"`
	MaxHandshakeBytes      int      `json:"max_handshake_bytes"`
}

type captureMetadata struct {
	SchemaVersion string            `json:"capture_schema_version"`
	Dependency    string            `json:"utls_version"`
	SpecSchema    string            `json:"spec_schema_version"`
	Policy        string            `json:"policy_version"`
	SpecSHA256    string            `json:"canonical_spec_sha256"`
	ProfileID     string            `json:"profile_identity"`
	Bun           bunRuntime        `json:"bun"`
	Conditions    captureConditions `json:"conditions"`
	SampleCount   int               `json:"sample_count"`
	Comparison    comparisonResult  `json:"comparison"`
	Limitations   []string          `json:"limitations"`
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "tls-clienthello:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, arguments []string, stdout io.Writer) error {
	if len(arguments) == 0 || arguments[0] == "help" || arguments[0] == "--help" || arguments[0] == "-h" {
		_, err := io.WriteString(stdout, usage)
		return err
	}
	command := arguments[0]
	if command != "capture" && command != "compare" {
		return fmt.Errorf("expected capture or compare; use --help for usage")
	}
	options, err := parseCommandOptions(command, arguments[1:])
	if errors.Is(err, flag.ErrHelp) {
		_, err = io.WriteString(stdout, usage)
		return err
	}
	if err != nil {
		return err
	}
	var outputs []outputFile
	var profile tlsprofile.Profile
	if command == "capture" {
		outputs, err = prepareOutputs(options.out, options.metadata, options.force)
	} else {
		var data []byte
		data, err = readSpec(options.spec)
		if err == nil {
			profile, err = tlsprofile.ParseCustom(data)
		}
	}
	if err != nil {
		return err
	}
	executable, err := resolveBun(ctx, options.bun)
	if err != nil {
		return err
	}
	runner := bunCapture{executable: executable, limits: defaultCaptureLimits()}
	captures := make([][]byte, 0, options.samples)
	var runtime bunRuntime
	var exported []byte
	for sample := 0; sample < options.samples; sample++ {
		raw, info, err := runner.capture(ctx)
		if err != nil {
			return fmt.Errorf("Bun sample %d: %w", sample+1, err)
		}
		if sample == 0 {
			runtime = info
		} else if info != runtime {
			return fmt.Errorf("Bun runtime metadata changed between fresh samples")
		}
		if command == "capture" {
			recipe, err := tlsprofile.ExportClientHello(raw)
			if err != nil {
				return fmt.Errorf("Bun sample %d: %w", sample+1, err)
			}
			if sample == 0 {
				exported = recipe
			} else if !bytes.Equal(exported, recipe) {
				return fmt.Errorf("Bun's exportable recipe changed between fresh samples")
			}
		}
		captures = append(captures, raw)
	}
	if command == "capture" {
		profile, err = tlsprofile.ParseCustom(exported)
		if err != nil {
			return fmt.Errorf("exported profile was not accepted: %w", err)
		}
	}
	comparison, err := compareProfile(ctx, captures, profile, defaultCaptureLimits())
	if err != nil {
		return err
	}
	metadata := newCaptureMetadata(runtime, profile, options.samples, comparison)
	encoded, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot encode capture metadata")
	}
	encoded = append(encoded, '\n')
	if command == "compare" {
		if _, err := stdout.Write(encoded); err != nil {
			return err
		}
	}
	if !comparison.Matched {
		return fmt.Errorf("normalized full-record comparison failed: %s", comparison.Differences[0])
	}
	if command == "capture" {
		outputs[0].data, outputs[1].data = exported, encoded
		if err := publishOutputs(outputs, options.force); err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "captured %d fresh Bun fetch samples; %d fresh uTLS samples matched (%d record bytes)\n", options.samples, options.samples, comparison.Structure.RecordBytes)
	}
	return err
}

func parseCommandOptions(command string, arguments []string) (commandOptions, error) {
	var options commandOptions
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&options.bun, "bun", "bun", "installed Bun executable")
	flags.StringVar(&options.api, "api", "fetch", "Bun networking API (fetch only)")
	flags.IntVar(&options.samples, "samples", 3, "fresh samples (2..16)")
	if command == "capture" {
		flags.StringVar(&options.out, "out", "", "loadable custom JSON output")
		flags.StringVar(&options.metadata, "metadata", "", "separate capture metadata output")
		flags.BoolVar(&options.force, "force", false, "replace existing regular output files")
	} else {
		flags.StringVar(&options.spec, "spec", "", "custom JSON to compare")
	}
	if err := flags.Parse(arguments); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return options, err
		}
		return options, fmt.Errorf("invalid command arguments; use --help for usage")
	}
	if flags.NArg() != 0 {
		return options, fmt.Errorf("unexpected positional arguments; use --help for usage")
	}
	if options.bun == "" {
		return options, fmt.Errorf("--bun must select an installed Bun executable")
	}
	if options.api != "fetch" {
		return options, fmt.Errorf("--api supports fetch only")
	}
	if options.samples < 2 || options.samples > 16 {
		return options, fmt.Errorf("--samples must be between 2 and 16")
	}
	if command == "capture" && (options.out == "" || options.metadata == "") {
		return options, fmt.Errorf("capture requires separate --out and --metadata paths")
	}
	if command == "compare" && options.spec == "" {
		return options, fmt.Errorf("compare requires --spec")
	}
	return options, nil
}

func newCaptureMetadata(runtime bunRuntime, profile tlsprofile.Profile, samples int, comparison comparisonResult) captureMetadata {
	digest := sha256.Sum256(profile.CanonicalJSON())
	return captureMetadata{
		SchemaVersion: "1", Dependency: tlsprofile.DependencyVersion,
		SpecSchema: tlsprofile.SchemaVersion, Policy: tlsprofile.PolicyVersion,
		SpecSHA256: fmt.Sprintf("%x", digest), ProfileID: profile.Identity(), Bun: runtime,
		Conditions: captureConditions{
			API: "fetch", FreshProcess: true, URL: "https://127.0.0.1:<ephemeral-port>/", ServerName: "localhost",
			Method: "GET", Redirect: "error", AbortMilliseconds: 4000, EmptyBunConfig: true,
			ExperimentalFlags: []string{}, IsolatedHomeAndCWD: true, StopAfterClientHello: true,
			AcceptReadMilliseconds: 3000, ChildMilliseconds: 5000, ChildOutputBytes: maxChildOutputBytes,
			MaxRecords: tlsprofile.MaxCaptureRecords, MaxRecordPayloadBytes: tlsprofile.MaxCaptureRecordBytes,
			MaxHandshakeBytes: tlsprofile.MaxCaptureHandshakeBytes,
		},
		SampleCount: samples, Comparison: comparison,
		Limitations: []string{
			"Measured installed Bun version and fetch API only; other versions, APIs, TLS options and SNI lengths are not covered.",
			"Fresh first-flight ClientHello only; resumed/PSK handshakes and unsupported extensions are rejected.",
			"Padding policy is not inferred; GREASE values are not normalized or guaranteed to match.",
			"Capture closes before certificates or HTTP traffic; successful verified TLS and serving activation require separate tests.",
			"Comparison preserves TLS record framing and all nonvolatile handshake bytes, not TCP segmentation or the HTTP stack.",
			"No provider networking, full Bun/browser impersonation, WAF acceptance or bot-detection bypass has been verified.",
		},
	}
}
