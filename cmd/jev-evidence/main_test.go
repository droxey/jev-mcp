package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestStatusAndLintWorkWithoutCredentials(t *testing.T) {
	for _, tt := range []struct {
		args     []string
		input    string
		code     int
		contains string
	}{
		{[]string{"status"}, "", 0, `"provider_configured":false`},
		{[]string{"lint"}, `{}`, 2, `"issues"`},
		{[]string{"lint"}, `not json`, 2, "invalid_plan_json"},
	} {
		var out, diagnostics bytes.Buffer
		code := run(t.Context(), tt.args, strings.NewReader(tt.input), &out, &diagnostics, func(string) string { return "" })
		if code != tt.code || !strings.Contains(out.String()+diagnostics.String(), tt.contains) {
			t.Fatalf("run=%d stdout=%q stderr=%q", code, out.String(), diagnostics.String())
		}
	}
}

func TestHTTPRequiresExplicitLocalAuthenticationPolicy(t *testing.T) {
	var out, diagnostics bytes.Buffer
	code := run(t.Context(), []string{"serve", "--transport", "http"}, strings.NewReader(""), &out, &diagnostics, func(string) string { return "" })
	if code == 0 || !strings.Contains(diagnostics.String(), "http_token_required_or_explicit_loopback_opt_out") {
		t.Fatalf("code=%d err=%s", code, diagnostics.String())
	}
	if out.Len() != 0 {
		t.Fatal("diagnostics polluted MCP stdout")
	}
}

func TestSecretIsNotDisclosedInConfigurationErrors(t *testing.T) {
	var out, diagnostics bytes.Buffer
	code := run(t.Context(), []string{"status"}, strings.NewReader(""), &out, &diagnostics, func(name string) string {
		if name == "TYPESAFE_API_KEY" {
			return "do-not-disclose-secret"
		}
		return ""
	})
	if code != 0 {
		t.Fatalf("status code %d", code)
	}
	if strings.Contains(out.String()+diagnostics.String(), "do-not-disclose-secret") {
		t.Fatal("credential disclosed")
	}
}

func TestModelDiscoveryConfigurationDoesNotRequireChosenModel(t *testing.T) {
	client, err := provider(func(name string) string {
		if name == "TYPESAFE_API_KEY" {
			return "fixture-key"
		}
		return ""
	})
	if err != nil || client == nil {
		t.Fatal("model discovery requires a selected model")
	}
}
