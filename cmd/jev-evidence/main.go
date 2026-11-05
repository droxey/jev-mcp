// Command jev-evidence serves the evidence MCP tools or checks local plans.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tphakala/jev-mcp/internal/evidence"
	"github.com/tphakala/jev-mcp/internal/mcpserver"
	"github.com/tphakala/jev-mcp/internal/typesafe"
)

const (
	maxPlanBytes          = 1 << 20
	exitUsage             = 2
	modelDiscoveryTimeout = 10 * time.Second
	defaultDeadline       = 90 * time.Second
	defaultMaxCalls       = 64
	defaultBatchSize      = 8
	defaultConcurrency    = 2
	maxCallsLimit         = 256
	maxBatchLimit         = 16
	maxConcurrencyLimit   = 8
	maxRetriesLimit       = 2
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Getenv)
	cancel()
	os.Exit(code)
}

func run(ctx context.Context, args []string, in io.Reader, out, diagnostics io.Writer, getenv func(string) string) int {
	if len(args) == 0 {
		args = []string{"serve"}
	}
	switch args[0] {
	case "status":
		d := configured(getenv)
		return writeJSON(out, mcpserver.Health(d))
	case "lint":
		return lint(args[1:], in, out, diagnostics)
	case "serve":
		return serve(ctx, args[1:], diagnostics, getenv)
	case "models":
		client, err := provider(getenv)
		if err != nil {
			_, _ = fmt.Fprintln(diagnostics, "provider_configuration_incomplete")
			return exitUsage
		}
		callCtx, cancel := context.WithTimeout(ctx, modelDiscoveryTimeout)
		defer cancel()
		models, err := client.Models(callCtx)
		if err != nil {
			_, _ = fmt.Fprintln(diagnostics, "provider_model_discovery_failed")
			return 1
		}
		return writeJSON(out, map[string]any{"models": models, "selected_automatically": false})
	default:
		_, _ = fmt.Fprintln(diagnostics, "unknown_command")
		return exitUsage
	}
}

func configured(getenv func(string) string) *mcpserver.Deps {
	d := &mcpserver.Deps{}
	if p, err := provider(getenv); err == nil {
		d.Model = getenv("JEV_EVIDENCE_MODEL")
		if d.Model != "" {
			d.Provider = p
		}
	}
	return d
}

func provider(getenv func(string) string) (*typesafe.Client, error) {
	return typesafe.New(typesafe.Config{APIKey: getenv("TYPESAFE_API_KEY"), Model: getenv("JEV_EVIDENCE_MODEL")})
}

func lint(args []string, in io.Reader, out, diagnostics io.Writer) int {
	flags := flag.NewFlagSet("lint", flag.ContinueOnError)
	flags.SetOutput(diagnostics)
	input := flags.String("input", "", "explicit plan JSON file; default stdin")
	if flags.Parse(args) != nil {
		return exitUsage
	}
	if *input != "" {
		f, err := os.Open(*input)
		if err != nil {
			_, _ = fmt.Fprintln(diagnostics, "plan_file_unavailable")
			return exitUsage
		}
		defer func() { _ = f.Close() }()
		in = f
	}
	p, err := evidence.DecodePlan(in)
	if err != nil {
		_, _ = fmt.Fprintln(diagnostics, "invalid_plan_json")
		return exitUsage
	}
	issues := evidence.Lint(&p)
	valid := true
	for _, issue := range issues {
		if issue.Severity == "error" {
			valid = false
		}
	}
	code := writeJSON(out, mcpserver.LintResult{Valid: valid, HeuristicOnly: true, Issues: issues, SourceHashes: evidence.SourceHashes(p)})
	if code != 0 {
		return code
	}
	if !valid {
		return exitUsage
	}
	return 0
}

func serve(ctx context.Context, args []string, diagnostics io.Writer, getenv func(string) string) int {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(diagnostics)
	transport := flags.String("transport", "stdio", "stdio or loopback http")
	listen := flags.String("listen", "127.0.0.1:8080", "loopback HTTP address")
	unauthenticated := flags.Bool("allow-unauthenticated-loopback", false, "explicitly permit local HTTP without bearer authentication")
	timeout := flags.Duration("timeout", defaultDeadline, "one total evaluation deadline, including retries")
	maxCalls := flags.Int("max-calls", defaultMaxCalls, "maximum primitive questions including retries")
	batchSize := flags.Int("batch-size", defaultBatchSize, "questions per independent provider batch, at most16")
	concurrency := flags.Int("concurrency", defaultConcurrency, "maximum concurrent provider batches")
	retries := flags.Int("retries", 0, "bounded provider retries included in max-calls")
	if flags.Parse(args) != nil {
		return exitUsage
	}
	if !validLimits(*timeout, *maxCalls, *batchSize, *concurrency, *retries) {
		_, _ = fmt.Fprintln(diagnostics, "invalid_execution_limits")
		return exitUsage
	}
	d := configured(getenv)
	d.Limits = evidence.Limits{Timeout: *timeout, MaxCalls: *maxCalls, BatchSize: *batchSize, Concurrency: *concurrency, MaxRetries: *retries, MaxInputBytes: maxPlanBytes}
	var err error
	switch *transport {
	case "stdio":
		err = mcpserver.ServeIO(ctx, d, os.Stdin, os.Stdout)
	case "http":
		err = mcpserver.ServeHTTP(ctx, d, *listen, mcpserver.HTTPOptions{Token: getenv("JEV_EVIDENCE_HTTP_TOKEN"), AllowUnauthenticatedLoopback: *unauthenticated})
	default:
		err = errors.New("invalid_transport")
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		_, _ = fmt.Fprintln(diagnostics, err.Error())
		return 1
	}
	return 0
}

func writeJSON(out io.Writer, value any) int {
	if err := json.NewEncoder(out).Encode(value); err != nil {
		return 1
	}
	return 0
}

func validLimits(timeout time.Duration, maxCalls, batchSize, concurrency, retries int) bool {
	return timeout > 0 && timeout <= defaultDeadline && maxCalls >= 1 && maxCalls <= maxCallsLimit && batchSize >= 1 && batchSize <= maxBatchLimit && concurrency >= 1 && concurrency <= maxConcurrencyLimit && retries >= 0 && retries <= maxRetriesLimit
}
