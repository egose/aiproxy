package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/egose/aiproxy/internal/copilotlogin"
	"github.com/spf13/cobra"
)

type loginCopilotOptions struct {
	ClientID    string
	Credential  string
	SecretsPath string
	Scope       string
}

func newLoginCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "login",
		Short:   "Authorize provider credentials via device flow",
		Long:    "Run an explicit device authorization and persist the credential locally. Login never edits HCL config and never signals a running server.",
		Example: "aiproxy login github-copilot --client-id Ov23li00000000000000 --credential copilot-main",
	}
	cmd.AddCommand(newLoginCopilotCommand())
	return cmd
}

func newLoginCopilotCommand() *cobra.Command {
	var opts loginCopilotOptions
	cmd := &cobra.Command{
		Use:   "github-copilot",
		Short: "Authorize with GitHub device flow and save a Copilot credential",
		Long: "Request GitHub device codes with an explicitly supplied OAuth client ID, wait for user authorization, " +
			"and persist a structured credential as a sidecar file next to the secrets path. " +
			"Requires SIGHUP or restart to activate. Re-run on 401/403.",
		Example: "aiproxy login github-copilot --client-id Ov23li00000000000000 --credential copilot-main\n" +
			"aiproxy login github-copilot --client-id Ov23li00000000000000 --credential copilot-main --secrets-path /etc/aiproxy/keys.json --scope read:user",
		RunE: func(cmd *cobra.Command, args []string) error {
			resolved := opts
			if resolved.SecretsPath == "" {
				resolved.SecretsPath = defaultSecretsPath()
			}
			if resolved.Scope == "" {
				resolved.Scope = copilotlogin.DefaultScope
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return runLoginCopilot(ctx, cmd.OutOrStdout(), cmd.ErrOrStderr(), resolved, nil)
		},
	}
	cmd.Flags().StringVar(&opts.ClientID, "client-id", "", "GitHub OAuth app client ID (required, public device client; no secret)")
	cmd.Flags().StringVar(&opts.Credential, "credential", "", "credential name for the saved login (required)")
	cmd.Flags().StringVar(&opts.SecretsPath, "secrets-path", "", "credential storage path (defaults to the secrets path)")
	cmd.Flags().StringVar(&opts.Scope, "scope", copilotlogin.DefaultScope, "OAuth scope for the device-code request")
	return cmd
}

func runLoginCopilot(ctx context.Context, stdout, stderr io.Writer, opts loginCopilotOptions, newClient func() *copilotlogin.Client) error {
	clientID := strings.TrimSpace(opts.ClientID)
	if clientID == "" {
		return fmt.Errorf("requires --client-id with an explicit GitHub OAuth app client ID")
	}
	if strings.ContainsAny(clientID, " \t\r\n") {
		return fmt.Errorf("invalid --client-id: must not contain whitespace")
	}
	credential := strings.TrimSpace(opts.Credential)
	if err := copilotlogin.ValidateCredentialName(credential); err != nil {
		if credential == "" {
			return fmt.Errorf("requires --credential with a credential name for the saved login")
		}
		return err
	}
	secretsPath := strings.TrimSpace(opts.SecretsPath)
	if secretsPath == "" {
		return fmt.Errorf("secrets path is required")
	}
	scope := strings.TrimSpace(opts.Scope)
	if scope == "" {
		scope = copilotlogin.DefaultScope
	}
	client := copilotlogin.New()
	if newClient != nil {
		if override := newClient(); override != nil {
			client = override
		}
	}
	code, err := client.RequestCode(ctx, clientID, scope)
	if err != nil {
		return mapLoginError(err)
	}
	fmt.Fprintf(stdout, "Open %s and enter code %s\n", code.VerificationURI, code.UserCode)
	fmt.Fprintln(stdout, "Waiting for authorization (Ctrl-C to cancel)...")
	deadline := time.Now().Add(code.ExpiresIn)
	token, err := client.Poll(ctx, clientID, code.DeviceCode, code.Interval, deadline)
	if err != nil {
		return mapLoginError(err)
	}
	cred, err := copilotlogin.NewCredential(clientID, token.AccessToken, time.Now())
	if err != nil {
		return mapLoginError(err)
	}
	destBefore := ""
	if dest, derr := copilotlogin.SidecarPath(secretsPath, credential); derr == nil {
		destBefore = dest
	}
	if err := copilotlogin.Save(secretsPath, credential, cred); err != nil {
		return fmt.Errorf("persist credential %q: %w", credential, err)
	}
	dest := destBefore
	if dest == "" {
		if resolved, derr := copilotlogin.SidecarPath(secretsPath, credential); derr == nil {
			dest = resolved
		} else {
			dest = secretsPath
		}
	}
	fmt.Fprintf(stdout, "saved credential %q to %s (0600)\n", credential, dest)
	fmt.Fprintln(stdout, "Not active until restart or SIGHUP. Re-run this login command on 401/403 or after revocation/expiry.")
	_ = stderr
	return nil
}

func mapLoginError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("login cancelled: %w", err)
	}
	msg := err.Error()
	if containsFold(msg, "context canceled") || containsFold(msg, "context deadline exceeded") {
		return fmt.Errorf("login cancelled: %w", err)
	}
	return err
}

func containsFold(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}
