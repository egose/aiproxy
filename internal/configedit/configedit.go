package configedit

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/egose/aiproxy/internal/filestore"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

var envExprPattern = regexp.MustCompile(`^env\("[^"]+"\)$`)

type TopLevelBlock struct {
	Type   string
	Labels []string
	Start  int
	End    int
	Text   string
}

type ListenerInput struct {
	Name       string
	Address    string
	ReadHeader string
	Idle       string
	Write      string
}

type AuthInput struct {
	Name      string
	Mode      string
	RateLimit *AuthRateLimitInput
	Clients   []AuthClientInput
}

type AuthRateLimitInput struct {
	RequestsPerMinute string
	Burst             string
}

type AuthClientInput struct {
	Name          string
	Token         string
	Tenant        string
	AllowedModels []string
}

type ProviderInput struct {
	ProviderType          string
	Name                  string
	DisplayName           string
	BaseURL               string
	UpstreamHeaderTimeout string
	Credential            ProviderCredentialInput
	Models                []ProviderModelInput
}

type ProviderCredentialInput struct {
	Mode        string
	APIKeyValue string
	SecretsPath string
	SecretsKey  string
}

type ProviderModelInput struct {
	Name         string
	DisplayName  string
	UpstreamName string
	Capabilities []string
}

type AliasInput struct {
	Name             string
	Algorithm        string
	RetryStatusCodes []string
	Targets          []AliasTargetInput
}

type AliasTargetInput struct {
	Provider string
	Model    string
}

type ProviderHealthInput struct {
	RedisURL  string
	KeyPrefix string
	Cooldown  string
}

type LoggingInput struct {
	Level     string
	AccessLog bool
}

type SecretsUpdate struct {
	Path  string
	Key   string
	Value string
}

func LoadDocument(path string) (string, []TopLevelBlock, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil, nil
		}
		return "", nil, fmt.Errorf("read config %s: %w", path, err)
	}
	blocks, err := ParseTopLevelBlocks(string(src))
	if err != nil {
		return "", nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	return string(src), blocks, nil
}

func HasBlockType(blocks []TopLevelBlock, blockType string) bool {
	for _, block := range blocks {
		if block.Type == blockType {
			return true
		}
	}
	return false
}

func FindBlock(blocks []TopLevelBlock, match func(TopLevelBlock) bool) *TopLevelBlock {
	for _, block := range blocks {
		if match(block) {
			copyBlock := block
			return &copyBlock
		}
	}
	return nil
}

func RenderListenerBlock(input ListenerInput) string {
	var b strings.Builder
	b.WriteString("listener \"http\" ")
	b.WriteString(strconv.Quote(input.Name))
	b.WriteString(" {\n")
	b.WriteString("  address = ")
	b.WriteString(strconv.Quote(input.Address))
	b.WriteString("\n")
	if input.ReadHeader != "" || input.Idle != "" || input.Write != "" {
		b.WriteString("\n  timeouts {\n")
		if input.ReadHeader != "" {
			b.WriteString("    read_header = ")
			b.WriteString(strconv.Quote(input.ReadHeader))
			b.WriteString("\n")
		}
		if input.Idle != "" {
			b.WriteString("    idle = ")
			b.WriteString(strconv.Quote(input.Idle))
			b.WriteString("\n")
		}
		if input.Write != "" {
			b.WriteString("    write = ")
			b.WriteString(strconv.Quote(input.Write))
			b.WriteString("\n")
		}
		b.WriteString("  }\n")
	}
	b.WriteString("}\n")
	return b.String()
}

func RenderAuthBlock(input AuthInput) string {
	var b strings.Builder
	b.WriteString("auth ")
	b.WriteString(strconv.Quote(input.Name))
	b.WriteString(" {\n")
	b.WriteString("  mode = ")
	b.WriteString(strconv.Quote(input.Mode))
	b.WriteString("\n")
	if input.RateLimit != nil {
		b.WriteString("\n  rate_limit {\n")
		b.WriteString("    requests_per_minute = ")
		b.WriteString(input.RateLimit.RequestsPerMinute)
		b.WriteString("\n")
		if input.RateLimit.Burst != "" {
			b.WriteString("    burst               = ")
			b.WriteString(input.RateLimit.Burst)
			b.WriteString("\n")
		}
		b.WriteString("  }\n")
	}
	for _, client := range input.Clients {
		b.WriteString("\n  client ")
		b.WriteString(strconv.Quote(client.Name))
		b.WriteString(" {\n")
		b.WriteString("    token = ")
		b.WriteString(RenderStringOrExpression(client.Token))
		b.WriteString("\n")
		if client.Tenant != "" {
			b.WriteString("    tenant = ")
			b.WriteString(strconv.Quote(client.Tenant))
			b.WriteString("\n")
		}
		if len(client.AllowedModels) > 0 {
			b.WriteString("    allowed_models = ")
			b.WriteString(RenderQuotedList(client.AllowedModels))
			b.WriteString("\n")
		}
		b.WriteString("  }\n")
	}
	b.WriteString("}\n")
	return b.String()
}

func RenderProviderBlock(input ProviderInput, defaultSecretsPath string) string {
	var b strings.Builder
	b.WriteString("provider ")
	b.WriteString(strconv.Quote(input.ProviderType))
	b.WriteString(" ")
	b.WriteString(strconv.Quote(input.Name))
	b.WriteString(" {\n")
	if input.DisplayName != "" {
		b.WriteString("  display_name = ")
		b.WriteString(strconv.Quote(input.DisplayName))
		b.WriteString("\n")
	}
	if input.BaseURL != "" {
		b.WriteString("  base_url = ")
		b.WriteString(strconv.Quote(input.BaseURL))
		b.WriteString("\n")
	}
	if input.UpstreamHeaderTimeout != "" {
		b.WriteString("  upstream_header_timeout = ")
		b.WriteString(strconv.Quote(input.UpstreamHeaderTimeout))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	switch input.Credential.Mode {
	case "secrets_file":
		b.WriteString("  api_key_ref {\n")
		if input.Credential.SecretsPath != defaultSecretsPath { // pragma: allowlist secret
			b.WriteString("    path = ")
			b.WriteString(strconv.Quote(input.Credential.SecretsPath))
			b.WriteString("\n")
		}
		b.WriteString("    key  = ")
		b.WriteString(strconv.Quote(input.Credential.SecretsKey))
		b.WriteString("\n")
		b.WriteString("  }\n")
	default:
		b.WriteString("  api_key = ")
		b.WriteString(RenderStringOrExpression(input.Credential.APIKeyValue))
		b.WriteString("\n")
	}
	for _, model := range input.Models {
		b.WriteString("\n  model ")
		b.WriteString(strconv.Quote(model.Name))
		b.WriteString(" {\n")
		if model.DisplayName != "" {
			b.WriteString("    display_name = ")
			b.WriteString(strconv.Quote(model.DisplayName))
			b.WriteString("\n")
		}
		if model.UpstreamName != "" && model.UpstreamName != model.Name {
			b.WriteString("    upstream_name = ")
			b.WriteString(strconv.Quote(model.UpstreamName))
			b.WriteString("\n")
		}
		if len(model.Capabilities) > 0 {
			b.WriteString("    capabilities = ")
			b.WriteString(RenderQuotedList(model.Capabilities))
			b.WriteString("\n")
		}
		b.WriteString("  }\n")
	}
	b.WriteString("}\n")
	return b.String()
}

func RenderAliasBlock(input AliasInput) string {
	var b strings.Builder
	b.WriteString("alias ")
	b.WriteString(strconv.Quote(input.Name))
	b.WriteString(" {\n")
	b.WriteString("  algorithm = ")
	b.WriteString(strconv.Quote(input.Algorithm))
	b.WriteString("\n")
	if len(input.RetryStatusCodes) > 0 {
		b.WriteString("  retry_status_codes = ")
		b.WriteString(RenderQuotedList(input.RetryStatusCodes))
		b.WriteString("\n")
	}
	for _, target := range input.Targets {
		b.WriteString("\n  target {\n")
		b.WriteString("    provider = ")
		b.WriteString(strconv.Quote(target.Provider))
		b.WriteString("\n")
		b.WriteString("    model    = ")
		b.WriteString(strconv.Quote(target.Model))
		b.WriteString("\n")
		b.WriteString("  }\n")
	}
	b.WriteString("}\n")
	return b.String()
}

func RenderProviderHealthBlock(input ProviderHealthInput) string {
	var b strings.Builder
	b.WriteString("provider_health {\n")
	if input.RedisURL != "" {
		b.WriteString("  redis_url = ")
		b.WriteString(strconv.Quote(input.RedisURL))
		b.WriteString("\n")
	}
	if input.KeyPrefix != "" {
		b.WriteString("  key_prefix = ")
		b.WriteString(strconv.Quote(input.KeyPrefix))
		b.WriteString("\n")
	}
	if input.Cooldown != "" {
		b.WriteString("  cooldown = ")
		b.WriteString(strconv.Quote(input.Cooldown))
		b.WriteString("\n")
	}
	b.WriteString("}\n")
	return b.String()
}

func RenderLoggingBlock(input LoggingInput) string {
	var b strings.Builder
	b.WriteString("logging {\n")
	b.WriteString("  level = ")
	b.WriteString(strconv.Quote(input.Level))
	b.WriteString("\n")
	b.WriteString("  access_log = ")
	b.WriteString(strconv.FormatBool(input.AccessLog))
	b.WriteString("\n")
	b.WriteString("}\n")
	return b.String()
}

func RenderStringOrExpression(value string) string {
	if envExprPattern.MatchString(value) {
		return value
	}
	return strconv.Quote(value)
}

func RenderQuotedList(values []string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.Quote(value))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func WriteConfigFile(path, source string) error {
	if err := ValidateGeneratedConfig([]byte(source), path); err != nil {
		return err
	}
	return WriteFile(path, []byte(source), 0o600)
}

func WriteProviderFiles(configPath, source string, update SecretsUpdate) error {
	if err := ValidateGeneratedConfig([]byte(source), configPath); err != nil {
		return err
	}
	if update.Path == "" {
		return WriteFile(configPath, []byte(source), 0o600)
	}
	secretsBody, err := BuildSecretsUpdate(update)
	if err != nil {
		return err
	}
	return filestore.ReplaceFiles([]filestore.File{
		{Path: update.Path, Data: secretsBody, Mode: 0o600},
		{Path: configPath, Data: []byte(source), Mode: 0o600},
	}, filestore.Options{DirMode: 0o700, Secret: true})
}

func WriteSecretsUpdate(update SecretsUpdate) error {
	if update.Path == "" {
		return nil
	}
	body, err := BuildSecretsUpdate(update)
	if err != nil {
		return err
	}
	return WriteFile(update.Path, body, 0o600)
}

func BuildSecretsUpdate(update SecretsUpdate) ([]byte, error) {
	secrets, err := ReadSecretsFile(update.Path)
	if err != nil {
		return nil, err
	}
	secrets[update.Key] = update.Value
	body, err := json.MarshalIndent(secrets, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal secrets %s: %w", update.Path, err)
	}
	body = append(body, '\n')
	return body, nil
}

func ReadSecretsFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("read secrets %s: %w", path, err)
	}
	var out map[string]string
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parse secrets %s: %w", path, err)
	}
	if out == nil {
		out = map[string]string{}
	}
	return out, nil
}

func WriteFile(path string, body []byte, mode os.FileMode) error {
	if err := filestore.WriteFile(path, body, mode, filestore.Options{DirMode: 0o700, Secret: true}); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func ValidateGeneratedConfig(source []byte, filename string) error {
	_, diags := hclsyntax.ParseConfig(source, filename, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return fmt.Errorf("parse config %s: %s", filename, diags.Error())
	}
	return nil
}

func UpsertBlock(source, blockText string, match func(TopLevelBlock) bool) (string, error) {
	if strings.TrimSpace(source) == "" {
		return strings.TrimRight(blockText, "\n") + "\n", nil
	}
	blocks, err := ParseTopLevelBlocks(source)
	if err != nil {
		return "", err
	}
	for _, block := range blocks {
		if match(block) {
			return strings.TrimRight(source[:block.Start], "\n") + "\n\n" + strings.TrimRight(blockText, "\n") + "\n" + strings.TrimLeft(source[block.End:], "\n"), nil
		}
	}
	trimmed := strings.TrimRight(source, "\n")
	if trimmed == "" {
		return strings.TrimRight(blockText, "\n") + "\n", nil
	}
	return trimmed + "\n\n" + strings.TrimRight(blockText, "\n") + "\n", nil
}

func RemoveBlock(source string, match func(TopLevelBlock) bool) (string, bool, error) {
	if strings.TrimSpace(source) == "" {
		return source, false, nil
	}
	blocks, err := ParseTopLevelBlocks(source)
	if err != nil {
		return "", false, err
	}
	for _, block := range blocks {
		if match(block) {
			prefix := strings.TrimRight(source[:block.Start], "\n")
			suffix := strings.TrimLeft(source[block.End:], "\n")
			switch {
			case prefix == "" && suffix == "":
				return "", true, nil
			case prefix == "":
				return suffix, true, nil
			case suffix == "":
				return prefix + "\n", true, nil
			default:
				return prefix + "\n\n" + suffix, true, nil
			}
		}
	}
	return source, false, nil
}

func UpsertTopLevelStringAttribute(source, name, value string) string {
	line := name + " = " + strconv.Quote(value)
	re := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(name) + `\s*=.*$`)
	if re.MatchString(source) {
		return re.ReplaceAllString(source, line)
	}
	trimmed := strings.TrimLeft(source, "\n")
	if strings.TrimSpace(trimmed) == "" {
		return line + "\n"
	}
	return line + "\n\n" + trimmed
}

func TopLevelStringAttribute(source, name string) string {
	re := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(name) + `\s*=\s*(.+?)\s*$`)
	match := re.FindStringSubmatch(source)
	if len(match) != 2 {
		return ""
	}
	value, err := strconv.Unquote(strings.TrimSpace(match[1]))
	if err != nil {
		return ""
	}
	return value
}

func ParseTopLevelBlocks(source string) ([]TopLevelBlock, error) {
	var blocks []TopLevelBlock
	for i := 0; i < len(source); {
		next, err := skipSpaceAndComments(source, i)
		if err != nil {
			return nil, err
		}
		i = next
		if i >= len(source) {
			break
		}
		if !isIdentStart(rune(source[i])) {
			return nil, fmt.Errorf("unexpected character %q at offset %d", source[i], i)
		}
		start := i
		typ, next := readIdentifier(source, i)
		i = next
		next, err = skipInlineSpace(source, i)
		if err != nil {
			return nil, err
		}
		if next < len(source) && source[next] == '=' {
			i, err = skipAttribute(source, next+1)
			if err != nil {
				return nil, err
			}
			_ = start
			_ = typ
			continue
		}
		var labels []string
		for {
			next, err = skipInlineSpace(source, i)
			if err != nil {
				return nil, err
			}
			i = next
			if i >= len(source) {
				return nil, fmt.Errorf("unexpected end of input after block header %q", typ)
			}
			if source[i] == '{' {
				break
			}
			if source[i] != '"' {
				return nil, fmt.Errorf("invalid block header for %q at offset %d", typ, i)
			}
			label, end, err := readQuotedString(source, i)
			if err != nil {
				return nil, err
			}
			labels = append(labels, label)
			i = end
		}
		end, err := scanBlockEnd(source, i)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, TopLevelBlock{Type: typ, Labels: labels, Start: start, End: end, Text: source[start:end]})
		i = end
	}
	return blocks, nil
}

func skipAttribute(source string, i int) (int, error) {
	inString := false
	escaped := false
	brackets := 0
	braces := 0
	for i < len(source) {
		ch := source[i]
		if inString {
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				inString = false
			}
			i++
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '[':
			brackets++
		case ']':
			if brackets > 0 {
				brackets--
			}
		case '{':
			braces++
		case '}':
			if braces > 0 {
				braces--
			}
		case '\n':
			if brackets == 0 && braces == 0 {
				return i + 1, nil
			}
		}
		i++
	}
	return i, nil
}

func ProviderBlockNames(blocks []TopLevelBlock) []string {
	var names []string
	for _, block := range blocks {
		if block.Type == "provider" && len(block.Labels) >= 2 {
			names = append(names, block.Labels[1])
		}
	}
	sort.Strings(names)
	return names
}

func AliasBlockNames(blocks []TopLevelBlock) []string {
	var names []string
	for _, block := range blocks {
		if block.Type == "alias" && len(block.Labels) >= 1 {
			names = append(names, block.Labels[0])
		}
	}
	sort.Strings(names)
	return names
}

func AvailableProviderModels(blocks []TopLevelBlock) []string {
	var out []string
	for _, block := range blocks {
		if block.Type != "provider" || len(block.Labels) < 2 {
			continue
		}
		providerName := block.Labels[1]
		for _, modelName := range ModelNamesFromProviderBlock(block.Text) {
			out = append(out, providerName+"/"+modelName)
		}
	}
	sort.Strings(out)
	return out
}

func AvailablePublicModels(blocks []TopLevelBlock) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(blocks))
	for _, item := range AvailableProviderModels(blocks) {
		if !seen[item] {
			out = append(out, item)
			seen[item] = true
		}
	}
	for _, aliasName := range AliasBlockNames(blocks) {
		item := "alias/" + aliasName
		if !seen[item] {
			out = append(out, item)
			seen[item] = true
		}
	}
	sort.Strings(out)
	return out
}

func ModelNamesFromProviderBlock(block string) []string {
	var names []string
	for _, match := range regexp.MustCompile(`(?m)^\s*model\s+"([^"]+)"\s*\{`).FindAllStringSubmatch(block, -1) {
		names = append(names, match[1])
	}
	return names
}

func skipSpaceAndComments(source string, i int) (int, error) {
	for i < len(source) {
		switch source[i] {
		case ' ', '\t', '\r', '\n':
			i++
		case '#':
			for i < len(source) && source[i] != '\n' {
				i++
			}
		case '/':
			if i+1 >= len(source) {
				return i, nil
			}
			switch source[i+1] {
			case '/':
				i += 2
				for i < len(source) && source[i] != '\n' {
					i++
				}
			case '*':
				end := strings.Index(source[i+2:], "*/")
				if end < 0 {
					return 0, fmt.Errorf("unterminated block comment at offset %d", i)
				}
				i += end + 4
			default:
				return i, nil
			}
		default:
			return i, nil
		}
	}
	return i, nil
}

func skipInlineSpace(source string, i int) (int, error) { return skipSpaceAndComments(source, i) }

func readIdentifier(source string, start int) (string, int) {
	i := start
	for i < len(source) && isIdentPart(rune(source[i])) {
		i++
	}
	return source[start:i], i
}

func readQuotedString(source string, start int) (string, int, error) {
	i := start + 1
	escaped := false
	for i < len(source) {
		if escaped {
			escaped = false
			i++
			continue
		}
		switch source[i] {
		case '\\':
			escaped = true
		case '"':
			value, err := strconv.Unquote(source[start : i+1])
			if err != nil {
				return "", 0, err
			}
			return value, i + 1, nil
		}
		i++
	}
	return "", 0, fmt.Errorf("unterminated string at offset %d", start)
}

func scanBlockEnd(source string, openBrace int) (int, error) {
	depth := 0
	inString := false
	escaped := false
	inLineComment := false
	inBlockComment := false
	for i := openBrace; i < len(source); i++ {
		ch := source[i]
		if inLineComment {
			if ch == '\n' {
				inLineComment = false
			}
			continue
		}
		if inBlockComment {
			if ch == '*' && i+1 < len(source) && source[i+1] == '/' {
				inBlockComment = false
				i++
			}
			continue
		}
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch ch {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		if ch == '"' {
			inString = true
			continue
		}
		if ch == '#' {
			inLineComment = true
			continue
		}
		if ch == '/' && i+1 < len(source) {
			switch source[i+1] {
			case '/':
				inLineComment = true
				i++
				continue
			case '*':
				inBlockComment = true
				i++
				continue
			}
		}
		switch ch {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i + 1, nil
			}
		}
	}
	return 0, fmt.Errorf("unterminated block starting at offset %d", openBrace)
}

func isIdentStart(r rune) bool { return unicode.IsLetter(r) || r == '_' }

func isIdentPart(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-'
}
