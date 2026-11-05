package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tphakala/jev-mcp/internal/evidence"
)

func TestToolsAvailableThroughStdioIO(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	sr, cw := io.Pipe()
	cr, sw := io.Pipe()
	done := make(chan error, 1)
	go func() { done <- ServeIO(ctx, &Deps{}, sr, sw) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.IOTransport{Reader: cr, Writer: cw}, nil)
	if err != nil {
		t.Fatal(err)
	}
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 3 {
		t.Fatalf("tools=%d", len(tools.Tools))
	}
	checkToolAnnotations(t, tools.Tools)
	status, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "status", Arguments: map[string]any{}})
	if err != nil || status.IsError {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	data, err := json.Marshal(status.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"provider_configured":false`) {
		t.Fatalf("status=%s", data)
	}
	if err := session.Close(); err != nil {
		t.Error(err)
	}
	cancel()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		t.Error(err)
	}
}

func checkToolAnnotations(t *testing.T, tools []*mcp.Tool) {
	t.Helper()
	wantAnnotations := map[string]struct{ idempotent, openWorld bool }{
		"status":        {idempotent: true, openWorld: false},
		"lint_plan":     {idempotent: true, openWorld: false},
		"evaluate_plan": {idempotent: false, openWorld: true},
	}
	for _, tool := range tools {
		t.Run(tool.Name+" annotations", func(t *testing.T) {
			want, ok := wantAnnotations[tool.Name]
			if !ok {
				t.Fatalf("unexpected tool %q", tool.Name)
			}
			got := tool.Annotations
			if got == nil {
				t.Fatal("tool annotations missing")
			}
			if !got.ReadOnlyHint || got.DestructiveHint == nil || *got.DestructiveHint {
				t.Errorf("tool must be explicitly read-only and non-destructive: %+v", got)
			}
			if got.IdempotentHint != want.idempotent || got.OpenWorldHint == nil || *got.OpenWorldHint != want.openWorld {
				t.Errorf("annotations=%+v want idempotent=%t openWorld=%t", got, want.idempotent, want.openWorld)
			}
		})
	}
}

func TestHTTPProtectionAndForwardingHeaders(t *testing.T) {
	handler, err := HTTPHandler(&Deps{}, HTTPOptions{Token: "local-secret"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, host, origin, token, forwarded string
		want                                 int
	}{
		{"missing auth", "localhost:8080", "", "", "", 401},
		{"host attack", "attacker.example", "", "local-secret", "localhost:8080", 403},
		{"origin attack", "localhost:8080", "https://attacker.example", "local-secret", "", 403},
		{"cross port", "localhost:8080", "http://localhost:9000", "local-secret", "", 403},
		{"forwarded not trusted", "localhost:8080", "", "local-secret", "attacker.example", 405},
		{"local", "localhost:8080", "http://localhost:8080", "local-secret", "", 405},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+tt.host+"/mcp", http.NoBody)
			req.Host = tt.host
			req.RemoteAddr = "127.0.0.1:3210"
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			if tt.token != "" {
				req.Header.Set("Authorization", "Bearer "+tt.token)
			}
			req.Header.Set("X-Forwarded-Host", tt.forwarded)
			rw := httptest.NewRecorder()
			handler.ServeHTTP(rw, req)
			if rw.Code != tt.want {
				t.Fatalf("status %d want %d body %s", rw.Code, tt.want, rw.Body.String())
			}
		})
	}
	if _, err := HTTPHandler(&Deps{}, HTTPOptions{}); err == nil {
		t.Error("unauthenticated HTTP not explicit")
	}
}

func TestStatelessHTTPMCPContract(t *testing.T) {
	handler, err := HTTPHandler(&Deps{}, HTTPOptions{AllowUnauthenticatedLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(handler)
	defer ts.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	c := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	s, err := c.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: ts.URL + "/mcp"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	result, err := s.CallTool(ctx, &mcp.CallToolParams{Name: "lint_plan", Arguments: evidence.Plan{}})
	if err != nil || result.IsError {
		t.Fatalf("lint: %+v %v", result, err)
	}
	if result.StructuredContent == nil {
		t.Error("structured output missing")
	}
}
