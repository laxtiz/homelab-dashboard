package probe

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"dashboard/internal/config"
)

func init() {
	Register("http", newHTTP)
}

type httpProbe struct {
	cfg config.ServiceConfig
	cli *http.Client
}

func newHTTP(cfg config.ServiceConfig) (Probe, error) {
	if cfg.URL == "" {
		return nil, errMissingURL
	}
	return &httpProbe{
		cfg: cfg,
		cli: &http.Client{Timeout: cfg.Timeout.Std()},
	}, nil
}

func (p *httpProbe) Type() string { return "http" }

func (p *httpProbe) Run(ctx context.Context) Result {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, p.cfg.Method, p.cfg.URL, nil)
	if err != nil {
		return Result{Status: "error", Err: err}
	}
	for k, v := range p.cfg.Headers {
		req.Header.Set(k, v)
	}

	resp, err := p.cli.Do(req)
	latency := float64(time.Since(start).Microseconds()) / 1000.0
	if err != nil {
		return Result{Status: "down", LatencyMS: latency, Err: err}
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	res := Result{
		Status:     "up",
		LatencyMS:  latency,
		Body:       string(body),
		HTTPStatus: resp.StatusCode,
		Headers:    map[string]string{},
	}
	for k := range resp.Header {
		res.Headers[k] = resp.Header.Get(k)
	}

	if p.cfg.Expect != "" && !strings.Contains(string(body), p.cfg.Expect) {
		res.Status = "down"
		res.Err = errExpectMismatch(p.cfg.Expect)
	}

	if looksLikeJSON(resp.Header.Get("Content-Type"), body) {
		if j, err := parseJSON(body); err == nil {
			res.JSON = j
		}
	}
	return res
}

func looksLikeJSON(contentType string, body []byte) bool {
	if strings.Contains(contentType, "json") {
		return true
	}
	t := strings.TrimSpace(string(body))
	return len(t) > 0 && (t[0] == '{' || t[0] == '[')
}

func parseJSON(body []byte) (any, error) {
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

func errExpectMismatch(expect string) error {
	return &ExpectError{Expect: expect}
}

// ExpectError indicates the response body did not contain the expected string.
type ExpectError struct{ Expect string }

func (e *ExpectError) Error() string {
	return "response does not contain expected value: " + e.Expect
}

var errMissingURL = &configError{msg: "http service requires url"}

type configError struct{ msg string }

func (e *configError) Error() string { return e.msg }
