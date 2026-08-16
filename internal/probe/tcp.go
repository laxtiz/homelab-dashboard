package probe

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"

	"dashboard/internal/config"
)

func init() {
	Register("tcp", newTCP)
	Register("udp", newUDP)
}

type streamProbe struct {
	cfg  config.ServiceConfig
	kind string
}

func newTCP(cfg config.ServiceConfig) (Probe, error) {
	if cfg.Address == "" {
		return nil, fmt.Errorf("tcp service requires address")
	}
	return &streamProbe{cfg: cfg, kind: "tcp"}, nil
}

func newUDP(cfg config.ServiceConfig) (Probe, error) {
	if cfg.Address == "" {
		return nil, fmt.Errorf("udp service requires address")
	}
	return &streamProbe{cfg: cfg, kind: "udp"}, nil
}

func (p *streamProbe) Type() string { return p.kind }

func (p *streamProbe) Run(ctx context.Context) Result {
	timeout := p.cfg.Timeout.Std()
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	start := time.Now()

	network := p.kind
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, network, p.cfg.Address)
	if err != nil {
		return Result{Status: "down", Err: fmt.Errorf("dial: %w", err)}
	}
	defer conn.Close()

	connectLatency := float64(time.Since(start).Microseconds()) / 1000.0

	payload, err := p.payload()
	if err != nil {
		return Result{Status: "error", Err: err}
	}
	if len(payload) > 0 {
		if _, err := conn.Write(payload); err != nil {
			return Result{Status: "down", LatencyMS: connectLatency, Err: fmt.Errorf("write: %w", err)}
		}
	}

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(timeout))
	}

	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	totalLatency := float64(time.Since(start).Microseconds()) / 1000.0
	if totalLatency < connectLatency {
		totalLatency = connectLatency
	}
	if err != nil && n == 0 && !isTimeoutOK(err) {
		return Result{Status: "down", LatencyMS: totalLatency, Err: fmt.Errorf("read: %w", err)}
	}

	res := Result{
		Status:    "up",
		LatencyMS: totalLatency,
		Body:      string(buf[:n]),
		Headers:   map[string]string{},
	}
	if res.Body != "" && (res.Body[0] == '{' || res.Body[0] == '[') {
		if j, err := parseJSON(buf[:n]); err == nil {
			res.JSON = j
		}
	}

	if p.cfg.Expect != "" && !matchesExpect(res.Body, p.cfg.Expect) {
		res.Status = "down"
		res.Err = errExpectMismatch(p.cfg.Expect)
	}
	return res
}

func (p *streamProbe) payload() ([]byte, error) {
	if p.cfg.PayloadB64 != "" {
		return base64.StdEncoding.DecodeString(p.cfg.PayloadB64)
	}
	return []byte(p.cfg.Payload), nil
}

func matchesExpect(body, expect string) bool {
	if len(expect) == 0 {
		return true
	}
	if expect[0] == '/' && expect[len(expect)-1] == '/' {
		return matchesRegex(body, expect[1:len(expect)-1])
	}
	return contains(body, expect)
}

func matchesRegex(body, pattern string) bool {
	ok, err := regexp.MatchString(pattern, body)
	return err == nil && ok
}

func isTimeoutOK(err error) bool {
	var ne net.Error
	if errors.As(err, &ne) {
		return ne.Timeout()
	}
	return false
}

func contains(s, sub string) bool {
	return len(sub) == 0 || strings.Contains(s, sub)
}