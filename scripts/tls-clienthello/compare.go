package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net"

	"github.com/egose/aiproxy/internal/tlsprofile"
	utls "github.com/refraction-networking/utls"
)

func compareProfile(ctx context.Context, bunCaptures [][]byte, profile tlsprofile.Profile, limits captureLimits) (comparisonResult, error) {
	result := comparisonResult{
		BunSamples: len(bunCaptures), Normalization: []string{"client_random_bytes", "session_id_bytes", "real_key_exchange_bytes"},
		Differences: []string{},
	}
	if len(bunCaptures) < 2 {
		return result, fmt.Errorf("comparison requires at least two fresh Bun samples")
	}
	baseline, err := tlsprofile.NormalizeClientHello(bunCaptures[0])
	if err != nil {
		return result, err
	}
	result.Structure, err = tlsprofile.SummarizeClientHello(baseline)
	if err != nil {
		return result, err
	}
	digest := sha256.Sum256(baseline)
	result.NormalizedSHA256 = fmt.Sprintf("%x", digest)
	for index, raw := range bunCaptures {
		normalized, err := tlsprofile.NormalizeClientHello(raw)
		if err != nil {
			return result, fmt.Errorf("Bun sample %d cannot be normalized: %w", index+1, err)
		}
		if difference := recordDifference(baseline, normalized); difference != "" {
			result.Differences = append(result.Differences, fmt.Sprintf("Bun sample %d: %s", index+1, difference))
		}
	}
	for index := range bunCaptures {
		raw, err := captureProfile(ctx, profile, limits)
		if err != nil {
			return result, fmt.Errorf("uTLS sample %d: %w", index+1, err)
		}
		result.UTLSSamples++
		normalized, err := tlsprofile.NormalizeClientHello(raw)
		if err != nil {
			return result, fmt.Errorf("uTLS sample %d cannot be normalized: %w", index+1, err)
		}
		if difference := recordDifference(baseline, normalized); difference != "" {
			result.Differences = append(result.Differences, fmt.Sprintf("uTLS sample %d: %s", index+1, difference))
		}
	}
	result.Matched = len(result.Differences) == 0
	return result, nil
}

func recordDifference(reference, measured []byte) string {
	if bytes.Equal(reference, measured) {
		return ""
	}
	for index := 0; index < len(reference) && index < len(measured); index++ {
		if reference[index] != measured[index] {
			return fmt.Sprintf("nonvolatile record bytes differ at offset %d (reference %d bytes, measured %d bytes)", index, len(reference), len(measured))
		}
	}
	return fmt.Sprintf("record lengths differ (reference %d bytes, measured %d bytes)", len(reference), len(measured))
}

func captureProfile(parent context.Context, profile tlsprofile.Profile, limits captureLimits) ([]byte, error) {
	ctx, cancel := context.WithTimeout(parent, limits.child)
	defer cancel()
	if err := captureContextError(ctx); err != nil {
		return nil, err
	}
	spec, err := profile.Materialize()
	if err != nil {
		return nil, err
	}
	listener, err := listenLoopback(ctx, limits.accept)
	if err != nil {
		return nil, err
	}
	defer listener.Close()
	stopListener := context.AfterFunc(ctx, func() { listener.Close() })
	defer stopListener()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer listener.Close()
		connection, err := (&net.Dialer{Timeout: limits.accept}).DialContext(ctx, "tcp4", listener.Addr().String())
		if err != nil {
			return
		}
		defer connection.Close()
		stopConnection := context.AfterFunc(ctx, func() { connection.Close() })
		defer stopConnection()
		config := &utls.Config{ServerName: "localhost", MinVersion: utls.VersionTLS12, MaxVersion: utls.VersionTLS13}
		client := utls.UClient(connection, config, utls.HelloCustom)
		if err := client.ApplyPreset(spec); err != nil {
			return
		}
		if config.MinVersion < utls.VersionTLS12 {
			config.MinVersion = utls.VersionTLS12
		}
		if config.MaxVersion > utls.VersionTLS13 {
			config.MaxVersion = utls.VersionTLS13
		}
		client.HandshakeContext(ctx)
	}()
	defer func() {
		cancel()
		<-done
	}()
	connection, err := listener.AcceptTCP()
	if err != nil {
		if contextErr := captureContextError(ctx); contextErr != nil {
			return nil, contextErr
		}
		return nil, fmt.Errorf("materialized profile did not connect to the loopback listener")
	}
	defer connection.Close()
	stopConnection := context.AfterFunc(ctx, func() { connection.Close() })
	defer stopConnection()
	if err := connection.SetReadDeadline(captureDeadline(ctx, limits.read)); err != nil {
		if contextErr := captureContextError(ctx); contextErr != nil {
			return nil, contextErr
		}
		return nil, fmt.Errorf("cannot set the generated ClientHello deadline")
	}
	raw, err := tlsprofile.ReadClientHello(connection)
	connection.Close()
	if err != nil {
		if contextErr := captureContextError(ctx); contextErr != nil {
			return nil, contextErr
		}
		return nil, fmt.Errorf("cannot read the generated ClientHello: %w", err)
	}
	if err := captureContextError(ctx); err != nil {
		return nil, err
	}
	return raw, nil
}
