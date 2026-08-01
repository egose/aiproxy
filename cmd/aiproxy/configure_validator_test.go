package main

import (
	"errors"
	"testing"
)

func TestValidateProviderName(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"primary", ""},
		{"primary-backup", ""},
		{"primary_2", ""},
		{"gpt4o.mini", ""},
		{"0openai", ""},
		{"Primary", "must start with [a-z0-9]"},
		{"primary/backup", "must start with [a-z0-9]"},
		{"primary backup", "must start with [a-z0-9]"},
		{"", "value is required"},
		{"   ", "value is required"},
	}
	for _, c := range cases {
		err := validateProviderName(c.in)
		checkValidatorError(t, c.in, err, c.want)
	}
}

func TestValidateAliasName(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"chat_default", ""},
		{"chat-default", ""},
		{"CHAT", "must start with [a-z0-9]"},
		{"chat/default", "must start with [a-z0-9]"},
		{"chat default", "must start with [a-z0-9]"},
		{"", "value is required"},
	}
	for _, c := range cases {
		err := validateAliasName(c.in)
		checkValidatorError(t, c.in, err, c.want)
	}
}

func TestValidateModelName(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"gpt-4o-mini", ""},
		{"text-embedding-3-small", ""},
		{"openai/gpt-4o-mini", ""},
		{"openai/gpt-4o-mini/best", ""},
		{"GPT-4o", "must start with [a-z0-9]"},
		{"gpt 4o", "must start with [a-z0-9]"},
		{"", "value is required"},
		{"openai/", "must start with [a-z0-9]"}, // empty final segment
		{"/gpt-4o", "must start with [a-z0-9]"}, // empty first segment
	}
	for _, c := range cases {
		err := validateModelName(c.in)
		checkValidatorError(t, c.in, err, c.want)
	}
}

func TestValidateDuration(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"30s", ""},
		{"2m", ""},
		{"1h", ""},
		{"100ms", ""},
		{"", ""}, // blank allowed (prompt treats blank as unset)
		{"30", "invalid duration"},
		{"thirty", "invalid duration"},
		{"-5s", "must not be negative"},
		{"1.5h", ""}, // ParseDuration accepts decimals
	}
	for _, c := range cases {
		err := validateDuration(c.in)
		checkValidatorError(t, c.in, err, c.want)
	}
}

func TestValidatePositiveInt(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"120", ""},
		{"1", ""},
		{"", ""}, // blank allowed (used for burst which defaults to rpm)
		{"0", "positive integer"},
		{"-1", "positive integer"},
		{"abc", "positive integer"},
		{"12.5", "positive integer"},
	}
	for _, c := range cases {
		err := validatePositiveInt(c.in)
		checkValidatorError(t, c.in, err, c.want)
	}
}

func TestValidateRequiredPositiveInt(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"120", ""},
		{"", "value is required"},
		{"0", "positive integer"},
		{"abc", "positive integer"},
	}
	for _, c := range cases {
		err := validateRequiredPositiveInt(c.in)
		checkValidatorError(t, c.in, err, c.want)
	}
}

func TestValidateEnvOrLiteral(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`env("OPENAI_API_KEY")`, ""},
		{"sk-test-12345", ""},
		{`env("FOO")`, ""},
		{`env(FOO)`, "must match env(\"VAR\")"},
		{`env("FOO"`, "must match env(\"VAR\")"},
		{`env("")`, "must match env(\"VAR\")"},
		{"", "value is required"},
	}
	for _, c := range cases {
		err := validateEnvOrLiteral(c.in)
		checkValidatorError(t, c.in, err, c.want)
	}
}

func checkValidatorError(t *testing.T, in string, err error, want string) {
	t.Helper()
	if want == "" {
		if err != nil {
			t.Errorf("validate(%q) got error %v, want nil", in, err)
		}
		return
	}
	if err == nil {
		t.Errorf("validate(%q) got nil error, want one containing %q", in, want)
		return
	}
	if !containsMessage(err.Error(), want) {
		t.Errorf("validate(%q) got error %q, want one containing %q", in, err.Error(), want)
	}
}

func containsMessage(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

var _ = errors.New
