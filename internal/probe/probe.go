package probe

import (
	"context"
	"fmt"
	"strings"

	"dashboard/internal/config"
	"dashboard/internal/dsl"
)

type Result struct {
	Status     string            `json:"-"` // up | down
	LatencyMS  float64           `json:"latency_ms"`
	Body       string            `json:"-"`
	Headers    map[string]string `json:"-"`
	HTTPStatus int               `json:"http_status"`
	JSON       any               `json:"-"`
	Extracted  map[string]any    `json:"-"`
	Err        error             `json:"-"`
}

func (r Result) Failed() bool { return r.Err != nil || r.Status == "down" }

type Probe interface {
	Type() string
	Run(ctx context.Context) Result
}

type Factory func(cfg config.ServiceConfig) (Probe, error)

var registry = map[string]Factory{}

func Register(typ string, f Factory) { registry[typ] = f }

func Build(cfg config.ServiceConfig) (Probe, error) {
	f, ok := registry[cfg.Type]
	if !ok {
		return nil, fmt.Errorf("service %q: unsupported probe type %q", cfg.Name, cfg.Type)
	}
	return f(cfg)
}

// Compiled probe: wraps a transport probe and applies DSL extraction.
type compiled struct {
	inner    Probe
	extracts []*dsl.Program
}

func (c *compiled) Type() string { return c.inner.Type() }

func (c *compiled) Run(ctx context.Context) Result {
	res := c.inner.Run(ctx)
	if len(c.extracts) == 0 {
		return res
	}
	env := dsl.Env{
		Status:       res.HTTPStatus,
		LatencyMS:    res.LatencyMS,
		ResponseSize: len(res.Body),
		Body:         res.Body,
		Headers:      res.Headers,
		JSON:         res.JSON,
	}
	extracted, err := dsl.EvalAll(env, c.extracts)
	if err != nil && res.Err == nil {
		res.Err = fmt.Errorf("extract: %w", err)
	}
	res.Extracted = extracted
	return res
}

// Compile builds a probe from a service config and compiles its DSL extract rules.
func Compile(cfg config.ServiceConfig) (Probe, error) {
	inner, err := Build(cfg)
	if err != nil {
		return nil, err
	}
	var programs []*dsl.Program
	for key, exprStr := range cfg.Extract {
		if strings.TrimSpace(exprStr) == "" {
			continue
		}
		p, err := dsl.Compile(key, exprStr)
		if err != nil {
			return nil, err
		}
		programs = append(programs, p)
	}
	if len(programs) == 0 {
		return inner, nil
	}
	return &compiled{inner: inner, extracts: programs}, nil
}