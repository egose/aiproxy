package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/filestore"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type adminClientOptions struct {
	configPath string
	server     string
	token      string
	output     string
}

func adminTokenFilePath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "aiproxy", "admin.token")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("aiproxy", "admin.token")
	}
	return filepath.Join(home, ".config", "aiproxy", "admin.token")
}

func resolveAdminToken(flagToken string) string {
	if strings.TrimSpace(flagToken) != "" {
		return strings.TrimSpace(flagToken)
	}
	if env := strings.TrimSpace(os.Getenv("AIPROXY_ADMIN_TOKEN")); env != "" {
		return env
	}
	data, err := os.ReadFile(adminTokenFilePath())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func persistAdminToken(token string) error {
	return filestore.WriteFile(adminTokenFilePath(), []byte(token+"\n"), 0o600, filestore.Options{DirMode: 0o700, Secret: true})
}

type adminEndpoint struct {
	baseURL string
	client  *http.Client
	token   string
}

func resolveAdminEndpoint(cmd *cobra.Command, opts *adminClientOptions) (*adminEndpoint, error) {
	baseURL := strings.TrimSpace(opts.server)
	if baseURL == "" {
		rt, err := config.LoadFileOrEnv(opts.configPath, cmd.Flags().Changed("config"))
		if err != nil {
			return nil, fmt.Errorf("config: %w", err)
		}
		if !rt.MultiTenancyEnabled() {
			return nil, errors.New("multi_tenancy not enabled: enable multi_tenancy + database first")
		}
		baseURL = normalizeBaseURL(rt.Listener.Address)
	}
	if err := validateDashboardTransport(baseURL); err != nil {
		return nil, err
	}
	return &adminEndpoint{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 15 * time.Second},
		token:   resolveAdminToken(opts.token),
	}, nil
}

func (e *adminEndpoint) do(ctx context.Context, method, path string, body interface{}) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, e.baseURL+path, reader)
	if err != nil {
		return 0, nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if e.token != "" {
		req.Header.Set("Authorization", "Bearer "+e.token)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("%w: %v", errTransport, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, raw, nil
}

func printAdminResponse(cmd *cobra.Command, opts *adminClientOptions, status int, raw []byte) error {
	if opts.output == "raw" {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(raw))
		return nil
	}
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(raw))
		return nil
	}
	pretty, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(pretty))
	if status >= 400 {
		return fmt.Errorf("server returned %d", status)
	}
	return nil
}

func adminDoAndPrint(cmd *cobra.Command, opts *adminClientOptions, method, path string, body interface{}, needToken bool) error {
	ep, err := resolveAdminEndpoint(cmd, opts)
	if err != nil {
		return err
	}
	if needToken && ep.token == "" {
		return errors.New("no admin token: pass --token, set AIPROXY_ADMIN_TOKEN, or run `aiproxy admin login`")
	}
	status, raw, err := ep.do(cmd.Context(), method, path, body)
	if err != nil {
		if isConnectionRefused(err) {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "no server running")
		}
		return err
	}
	if status == http.StatusNotFound && strings.HasPrefix(path, "/_internal/admin/") && path != "/_internal/admin/status" {
		var v map[string]interface{}
		_ = json.Unmarshal(raw, &v)
	}
	return printAdminResponse(cmd, opts, status, raw)
}

func newAdminCommand() *cobra.Command {
	opts := &adminClientOptions{}
	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Manage multi-tenancy resources via the running server (requires multi_tenancy)",
	}
	cmd.PersistentFlags().StringVarP(&opts.configPath, "config", "c", defaultConfigPath(), "path to config file (overrides $AIPROXY_CONFIG)")
	cmd.PersistentFlags().StringVar(&opts.server, "server", "", "server base URL (defaults to the listener address in config)")
	cmd.PersistentFlags().StringVar(&opts.token, "token", "", "admin JWT (defaults to $AIPROXY_ADMIN_TOKEN or the login token file)")
	cmd.PersistentFlags().StringVarP(&opts.output, "output", "o", "json", "output format: json or raw")
	cmd.AddCommand(newAdminLoginCommand(opts))
	cmd.AddCommand(newAdminLogoutCommand(opts))
	cmd.AddCommand(newAdminStatusCommand(opts))
	cmd.AddCommand(newAdminProvidersCommand(opts))
	cmd.AddCommand(newAdminAliasesCommand(opts))
	cmd.AddCommand(newAdminKeysCommand(opts))
	cmd.AddCommand(newAdminUsersCommand(opts))
	cmd.AddCommand(newAdminOIDCCommand(opts))
	return cmd
}

func newAdminLoginCommand(opts *adminClientOptions) *cobra.Command {
	var email, password string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Sign in with admin credentials and save the session token",
		RunE: func(cmd *cobra.Command, args []string) error {
			if email == "" {
				return errors.New("requires --email")
			}
			if password == "" {
				if !term.IsTerminal(int(os.Stdin.Fd())) {
					return errors.New("requires --password (or a terminal for interactive entry)")
				}
				_, _ = fmt.Fprint(cmd.ErrOrStderr(), "Password: ")
				raw, err := term.ReadPassword(int(os.Stdin.Fd()))
				_, _ = fmt.Fprintln(cmd.ErrOrStderr())
				if err != nil {
					return fmt.Errorf("read password: %w", err)
				}
				password = string(raw)
			}
			ep, err := resolveAdminEndpoint(cmd, opts)
			if err != nil {
				return err
			}
			status, raw, err := ep.do(cmd.Context(), http.MethodPost, "/_internal/admin/login", map[string]string{"email": email, "password": password})
			if err != nil {
				if isConnectionRefused(err) {
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "no server running")
				}
				return err
			}
			if status != http.StatusOK {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), string(raw))
				return fmt.Errorf("login failed with status %d", status)
			}
			var out struct {
				AccessToken  string `json:"access_token"`
				RefreshToken string `json:"refresh_token"`
			}
			if err := json.Unmarshal(raw, &out); err != nil {
				return err
			}
			if err := persistAdminToken(out.AccessToken); err != nil {
				return err
			}
			if refreshPath := os.Getenv("AIPROXY_ADMIN_REFRESH_FILE"); refreshPath != "" {
				_ = filestore.WriteFile(refreshPath, []byte(out.RefreshToken+"\n"), 0o600, filestore.Options{DirMode: 0o700, Secret: true})
			} else {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "refresh token not persisted (set AIPROXY_ADMIN_REFRESH_FILE to persist it)")
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "logged in")
			return nil
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "admin email (or $AIPROXY_ADMIN_EMAIL)")
	cmd.Flags().StringVar(&password, "password", "", "admin password (or terminal prompt)")
	if email == "" {
		email = os.Getenv("AIPROXY_ADMIN_EMAIL")
	}
	return cmd
}

func newAdminLogoutCommand(opts *adminClientOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Discard the saved admin session token",
		RunE: func(cmd *cobra.Command, args []string) error {
			ep, err := resolveAdminEndpoint(cmd, opts)
			if err != nil {
				return err
			}
			if ep.token != "" {
				_, _, _ = ep.do(cmd.Context(), http.MethodPost, "/_internal/admin/logout", map[string]string{})
			}
			_ = os.Remove(adminTokenFilePath())
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "logged out")
			return nil
		},
	}
}

func newAdminStatusCommand(opts *adminClientOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show multi-tenancy and database status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return adminDoAndPrint(cmd, opts, http.MethodGet, "/_internal/admin/status", nil, false)
		},
	}
}

func newAdminProvidersCommand(opts *adminClientOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "providers", Short: "Manage database providers"}
	cmd.AddCommand(
		&cobra.Command{Use: "list", Short: "List merged config + database providers", RunE: func(cmd *cobra.Command, args []string) error {
			return adminDoAndPrint(cmd, opts, http.MethodGet, "/_internal/admin/providers", nil, true)
		}},
		&cobra.Command{Use: "get NAME", Short: "Show one provider", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			return adminDoAndPrint(cmd, opts, http.MethodGet, "/_internal/admin/providers/"+args[0], nil, true)
		}},
		&cobra.Command{Use: "delete NAME", Short: "Delete a database provider (config providers are protected)", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			return adminDoAndPrint(cmd, opts, http.MethodDelete, "/_internal/admin/providers/"+args[0], nil, true)
		}},
	)
	createCmd := &cobra.Command{Use: "create", Short: "Create a database provider from a JSON file"}
	var file, name, providerType, baseURL, apiKey, modelsCSV string
	var enabled bool
	createCmd.Flags().StringVar(&file, "file", "", "JSON body file (takes precedence over flags)")
	createCmd.Flags().StringVar(&name, "name", "", "provider name")
	createCmd.Flags().StringVar(&providerType, "type", "", "provider type")
	createCmd.Flags().StringVar(&baseURL, "base-url", "", "base URL override")
	createCmd.Flags().StringVar(&apiKey, "api-key", "", "upstream API key (or $AIPROXY_ADMIN_API_KEY)")
	createCmd.Flags().StringVar(&modelsCSV, "models", "", "comma-separated model names")
	createCmd.Flags().BoolVar(&enabled, "enabled", true, "enable the provider")
	createCmd.RunE = func(cmd *cobra.Command, args []string) error {
		var body interface{}
		if file != "" {
			raw, err := os.ReadFile(file)
			if err != nil {
				return err
			}
			var v interface{}
			if err := json.Unmarshal(raw, &v); err != nil {
				return err
			}
			body = v
		} else {
			if name == "" || providerType == "" {
				return errors.New("requires --name and --type (or --file)")
			}
			key := apiKey
			if key == "" {
				key = os.Getenv("AIPROXY_ADMIN_API_KEY")
			}
			models := []map[string]string{}
			for _, m := range strings.Split(modelsCSV, ",") {
				if trimmed := strings.TrimSpace(m); trimmed != "" {
					models = append(models, map[string]string{"name": trimmed})
				}
			}
			body = map[string]interface{}{"name": name, "type": providerType, "base_url": baseURL, "enabled": enabled, "api_key": key, "models": models}
		}
		return adminDoAndPrint(cmd, opts, http.MethodPost, "/_internal/admin/providers", body, true)
	}
	cmd.AddCommand(createCmd)
	credCmd := &cobra.Command{Use: "set-credential NAME", Short: "Set a database provider upstream credential", Args: cobra.ExactArgs(1)}
	var credKey string
	credCmd.Flags().StringVar(&credKey, "api-key", "", "upstream API key (or $AIPROXY_ADMIN_API_KEY)")
	credCmd.RunE = func(cmd *cobra.Command, args []string) error {
		key := credKey
		if key == "" {
			key = os.Getenv("AIPROXY_ADMIN_API_KEY")
		}
		if key == "" {
			return errors.New("requires --api-key")
		}
		return adminDoAndPrint(cmd, opts, http.MethodPut, "/_internal/admin/providers/"+args[0]+"/credential", map[string]string{"api_key": key}, true)
	}
	cmd.AddCommand(credCmd)
	return cmd
}

func newAdminAliasesCommand(opts *adminClientOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "aliases", Short: "Manage database aliases"}
	cmd.AddCommand(
		&cobra.Command{Use: "list", Short: "List merged config + database aliases", RunE: func(cmd *cobra.Command, args []string) error {
			return adminDoAndPrint(cmd, opts, http.MethodGet, "/_internal/admin/aliases", nil, true)
		}},
		&cobra.Command{Use: "get NAME", Short: "Show one alias", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			return adminDoAndPrint(cmd, opts, http.MethodGet, "/_internal/admin/aliases/"+args[0], nil, true)
		}},
		&cobra.Command{Use: "delete NAME", Short: "Delete a database alias (config aliases are protected)", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			return adminDoAndPrint(cmd, opts, http.MethodDelete, "/_internal/admin/aliases/"+args[0], nil, true)
		}},
	)
	createCmd := &cobra.Command{Use: "create", Short: "Create a database alias from a JSON file or flags"}
	var file, name, algorithm, targetsCSV, providersCSV, shorthandModel, retryCodesCSV string
	createCmd.Flags().StringVar(&file, "file", "", "JSON body file (takes precedence over flags)")
	createCmd.Flags().StringVar(&name, "name", "", "alias name")
	createCmd.Flags().StringVar(&algorithm, "algorithm", "round_robin", "round_robin or least_connections")
	createCmd.Flags().StringVar(&targetsCSV, "targets", "", "comma-separated provider/model targets")
	createCmd.Flags().StringVar(&providersCSV, "providers", "", "shorthand: comma-separated providers sharing --model (cannot combine with --targets)")
	createCmd.Flags().StringVar(&shorthandModel, "model", "", "shorthand model shared across --providers")
	createCmd.Flags().StringVar(&retryCodesCSV, "retry-codes", "", "comma-separated retry status codes (default 500,502,503,504)")
	createCmd.RunE = func(cmd *cobra.Command, args []string) error {
		var body interface{}
		if file != "" {
			raw, err := os.ReadFile(file)
			if err != nil {
				return err
			}
			var v interface{}
			if err := json.Unmarshal(raw, &v); err != nil {
				return err
			}
			body = v
		} else {
			if name == "" {
				return errors.New("requires --name (or --file)")
			}
			payload := map[string]interface{}{"name": name, "algorithm": algorithm}
			if retryCodesCSV != "" {
				codes := []int{}
				for _, s := range strings.Split(retryCodesCSV, ",") {
					var code int
					if _, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &code); err != nil {
						return fmt.Errorf("invalid retry code %q: expected integer", s)
					}
					codes = append(codes, code)
				}
				payload["retry_status_codes"] = codes
			}
			if providersCSV != "" || shorthandModel != "" {
				providers := []string{}
				for _, p := range strings.Split(providersCSV, ",") {
					if trimmed := strings.TrimSpace(p); trimmed != "" {
						providers = append(providers, trimmed)
					}
				}
				payload["providers"] = providers
				payload["model"] = shorthandModel
			} else {
				if targetsCSV == "" {
					return errors.New("requires --targets or --providers with --model (or --file)")
				}
				targets := []map[string]string{}
				for _, t := range strings.Split(targetsCSV, ",") {
					parts := strings.SplitN(strings.TrimSpace(t), "/", 2)
					if len(parts) != 2 {
						return fmt.Errorf("invalid target %q (want provider/model)", t)
					}
					targets = append(targets, map[string]string{"provider": parts[0], "model": parts[1]})
				}
				payload["targets"] = targets
			}
			body = payload
		}
		return adminDoAndPrint(cmd, opts, http.MethodPost, "/_internal/admin/aliases", body, true)
	}
	cmd.AddCommand(createCmd)
	return cmd
}

func newAdminKeysCommand(opts *adminClientOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "keys", Short: "Manage inbound API keys"}
	cmd.AddCommand(
		&cobra.Command{Use: "list", Short: "List config + database API keys", RunE: func(cmd *cobra.Command, args []string) error {
			return adminDoAndPrint(cmd, opts, http.MethodGet, "/_internal/admin/keys", nil, true)
		}},
		&cobra.Command{Use: "rotate ID", Short: "Rotate a database key (prints the new token)", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			return adminDoAndPrint(cmd, opts, http.MethodPost, "/_internal/admin/keys/"+args[0]+"/rotate", map[string]string{}, true)
		}},
		&cobra.Command{Use: "revoke ID", Short: "Revoke a database key", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			return adminDoAndPrint(cmd, opts, http.MethodPost, "/_internal/admin/keys/"+args[0]+"/revoke", map[string]string{}, true)
		}},
		&cobra.Command{Use: "delete ID", Short: "Delete a database key", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			return adminDoAndPrint(cmd, opts, http.MethodDelete, "/_internal/admin/keys/"+args[0], nil, true)
		}},
	)
	createCmd := &cobra.Command{Use: "create", Short: "Issue a database API key"}
	var name, tenant, allowedCSV string
	createCmd.Flags().StringVar(&name, "name", "", "key name")
	createCmd.Flags().StringVar(&tenant, "tenant", "", "tenant label")
	createCmd.Flags().StringVar(&allowedCSV, "allowed-models", "", "comma-separated allowed models (empty = all)")
	createCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if name == "" {
			return errors.New("requires --name")
		}
		allowed := []string{}
		for _, m := range strings.Split(allowedCSV, ",") {
			if trimmed := strings.TrimSpace(m); trimmed != "" {
				allowed = append(allowed, trimmed)
			}
		}
		return adminDoAndPrint(cmd, opts, http.MethodPost, "/_internal/admin/keys", map[string]interface{}{"name": name, "tenant": tenant, "allowed_models": allowed}, true)
	}
	cmd.AddCommand(createCmd)
	return cmd
}

func newAdminUsersCommand(opts *adminClientOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "users", Short: "Manage users and roles"}
	cmd.AddCommand(
		&cobra.Command{Use: "list", Short: "List users", RunE: func(cmd *cobra.Command, args []string) error {
			return adminDoAndPrint(cmd, opts, http.MethodGet, "/_internal/admin/users", nil, true)
		}},
	)
	createCmd := &cobra.Command{Use: "create", Short: "Create a user"}
	var email, password, role string
	var isAdmin bool
	createCmd.Flags().StringVar(&email, "email", "", "user email")
	createCmd.Flags().StringVar(&password, "password", "", "password (or terminal prompt)")
	createCmd.Flags().StringVar(&role, "role", "", "global role: admin or user")
	createCmd.Flags().BoolVar(&isAdmin, "admin", false, "grant admin role (deprecated: use --role admin)")
	createCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if email == "" {
			return errors.New("requires --email")
		}
		if password == "" {
			if !term.IsTerminal(int(os.Stdin.Fd())) {
				return errors.New("requires --password (or a terminal for interactive entry)")
			}
			_, _ = fmt.Fprint(cmd.ErrOrStderr(), "Password: ")
			raw, err := term.ReadPassword(int(os.Stdin.Fd()))
			_, _ = fmt.Fprintln(cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			password = string(raw)
		}
		body := map[string]interface{}{"email": email, "password": password}
		if cmd.Flags().Changed("role") {
			body["role"] = role
		} else {
			body["is_admin"] = isAdmin
		}
		return adminDoAndPrint(cmd, opts, http.MethodPost, "/_internal/admin/users", body, true)
	}
	cmd.AddCommand(createCmd)
	for _, action := range []string{"disable", "enable"} {
		action := action
		cmd.AddCommand(&cobra.Command{Use: action + " ID", Short: action + " a user", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			return adminDoAndPrint(cmd, opts, http.MethodPost, "/_internal/admin/users/"+args[0]+"/"+action, map[string]string{}, true)
		}})
	}
	roleCmd := &cobra.Command{Use: "role ID --role admin|user", Short: "Change a user's global role", Args: cobra.ExactArgs(1)}
	var newRole string
	roleCmd.Flags().StringVar(&newRole, "role", "", "global role: admin or user")
	roleCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if newRole == "" {
			return errors.New("requires --role admin|user")
		}
		return adminDoAndPrint(cmd, opts, http.MethodPost, "/_internal/admin/users/"+args[0]+"/role", map[string]string{"role": newRole}, true)
	}
	cmd.AddCommand(roleCmd)
	resetCmd := &cobra.Command{Use: "reset-password ID", Short: "Reset a user's password", Args: cobra.ExactArgs(1)}
	var newPassword string
	resetCmd.Flags().StringVar(&newPassword, "password", "", "new password (or terminal prompt)")
	resetCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if newPassword == "" {
			if !term.IsTerminal(int(os.Stdin.Fd())) {
				return errors.New("requires --password (or a terminal for interactive entry)")
			}
			_, _ = fmt.Fprint(cmd.ErrOrStderr(), "New password: ")
			raw, err := term.ReadPassword(int(os.Stdin.Fd()))
			_, _ = fmt.Fprintln(cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			newPassword = string(raw)
		}
		return adminDoAndPrint(cmd, opts, http.MethodPost, "/_internal/admin/users/"+args[0]+"/reset-password", map[string]string{"password": newPassword}, true)
	}
	cmd.AddCommand(resetCmd)
	cmd.AddCommand(&cobra.Command{Use: "delete ID", Short: "Delete a user", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return adminDoAndPrint(cmd, opts, http.MethodDelete, "/_internal/admin/users/"+args[0], nil, true)
	}})
	inviteCmd := &cobra.Command{Use: "invite", Short: "Invite a user (prints a one-time token)"}
	var inviteEmail, inviteRole string
	inviteCmd.Flags().StringVar(&inviteEmail, "email", "", "invitee email")
	inviteCmd.Flags().StringVar(&inviteRole, "role", "user", "global role: admin or user")
	inviteCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if inviteEmail == "" {
			return errors.New("requires --email")
		}
		return adminDoAndPrint(cmd, opts, http.MethodPost, "/_internal/admin/invites", map[string]string{"email": inviteEmail, "role": inviteRole}, true)
	}
	cmd.AddCommand(inviteCmd)
	cmd.AddCommand(&cobra.Command{Use: "invites", Short: "List pending invites", RunE: func(cmd *cobra.Command, args []string) error {
		return adminDoAndPrint(cmd, opts, http.MethodGet, "/_internal/admin/invites", nil, true)
	}})
	cmd.AddCommand(&cobra.Command{Use: "invite-revoke ID", Short: "Revoke a pending invite", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return adminDoAndPrint(cmd, opts, http.MethodDelete, "/_internal/admin/invites/"+args[0], nil, true)
	}})
	return cmd
}

func newAdminOIDCCommand(opts *adminClientOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "oidc-config", Short: "Manage single sign-on (OIDC) settings"}
	cmd.AddCommand(&cobra.Command{Use: "get", Short: "Show OIDC settings (secret never displayed)", RunE: func(cmd *cobra.Command, args []string) error {
		return adminDoAndPrint(cmd, opts, http.MethodGet, "/_internal/admin/oidc-config", nil, true)
	}})
	setCmd := &cobra.Command{Use: "set", Short: "Update OIDC settings from flags or a JSON file"}
	var file, issuer, clientID, clientSecret, scopes, usernameClaim, adminClaim, adminValue, displayName string
	var enabled, disabled bool
	setCmd.Flags().StringVar(&file, "file", "", "JSON body file (takes precedence over flags)")
	setCmd.Flags().StringVar(&issuer, "issuer", "", "OIDC issuer URL")
	setCmd.Flags().StringVar(&clientID, "client-id", "", "OIDC client ID")
	setCmd.Flags().StringVar(&clientSecret, "client-secret", "", "OIDC client secret (or $AIPROXY_OIDC_CLIENT_SECRET)")
	setCmd.Flags().StringVar(&scopes, "scopes", "", "space-separated scopes")
	setCmd.Flags().StringVar(&usernameClaim, "username-claim", "", "username claim")
	setCmd.Flags().StringVar(&adminClaim, "admin-claim", "", "admin role claim (empty clears)")
	setCmd.Flags().StringVar(&adminValue, "admin-value", "", "admin role value (empty clears)")
	setCmd.Flags().StringVar(&displayName, "display-name", "", "login button label")
	setCmd.Flags().BoolVar(&enabled, "enabled", false, "enable single sign-on")
	setCmd.Flags().BoolVar(&disabled, "disabled", false, "disable single sign-on")
	setCmd.RunE = func(cmd *cobra.Command, args []string) error {
		var body interface{}
		if file != "" {
			raw, err := os.ReadFile(file)
			if err != nil {
				return err
			}
			var v interface{}
			if err := json.Unmarshal(raw, &v); err != nil {
				return err
			}
			body = v
		} else {
			secret := clientSecret // pragma: allowlist secret
			if secret == "" {
				secret = os.Getenv("AIPROXY_OIDC_CLIENT_SECRET")
			}
			payload := map[string]interface{}{}
			if cmd.Flags().Changed("enabled") {
				payload["enabled"] = true
			}
			if disabled {
				payload["enabled"] = false
			}
			if issuer != "" {
				payload["issuer_url"] = issuer
			}
			if clientID != "" {
				payload["client_id"] = clientID
			}
			if secret != "" {
				payload["client_secret"] = secret // pragma: allowlist secret
			}
			if scopes != "" {
				payload["scopes"] = scopes
			}
			if usernameClaim != "" {
				payload["username_claim"] = usernameClaim
			}
			if cmd.Flags().Changed("admin-claim") {
				payload["admin_claim"] = adminClaim
			}
			if cmd.Flags().Changed("admin-value") {
				payload["admin_value"] = adminValue
			}
			if displayName != "" {
				payload["display_name"] = displayName
			}
			if len(payload) == 0 {
				return errors.New("nothing to set: pass --file or at least one flag")
			}
			body = payload
		}
		return adminDoAndPrint(cmd, opts, http.MethodPut, "/_internal/admin/oidc-config", body, true)
	}
	cmd.AddCommand(setCmd)
	return cmd
}
