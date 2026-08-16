package dsl

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"github.com/ohler55/ojg/jp"
)

// Env is the environment exposed to DSL expressions for each probe run.
type Env struct {
	Status       int               `expr:"status"`
	LatencyMS    float64           `expr:"latency_ms"`
	ResponseSize int               `expr:"response_size"`
	Body         string            `expr:"body"`
	Headers      map[string]string `expr:"headers"`
	JSON         any               `expr:"json"`
}

func (Env) Jsonpath(v any, path string) any {
	if v == nil {
		return nil
	}
	x, err := jp.ParseString(path)
	if err != nil {
		return nil
	}
	out := x.Get(v)
	if len(out) == 0 {
		return nil
	}
	return out[0]
}

func (Env) Regex(s string, pattern string) any {
	if pattern == "" {
		return nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil
	}
	m := re.FindStringSubmatch(s)
	if len(m) == 0 {
		return nil
	}
	if len(m) > 1 {
		return m[1]
	}
	return m[0]
}

func (Env) Match(s string, pattern string) bool {
	ok, _ := regexp.MatchString(pattern, s)
	return ok
}

func (Env) Round(x float64, n int) float64 {
	p := pow10(n)
	return float64(int(x*p+0.5)) / p
}

func (Env) Int(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case string:
		i, _ := strconv.Atoi(t)
		return i
	case bool:
		if t {
			return 1
		}
	}
	return 0
}

func (Env) Float(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case string:
		f, _ := strconv.ParseFloat(t, 64)
		return f
	case bool:
		if t {
			return 1
		}
	}
	return 0
}

func (Env) Str(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int:
		return strconv.Itoa(t)
	case bool:
		return strconv.FormatBool(t)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", t)
	}
}

func pow10(n int) float64 {
	p := 1.0
	for i := 0; i < n; i++ {
		p *= 10
	}
	return p
}

// Program is a pre-compiled expression tied to an output key.
type Program struct {
	key  string
	prog *vm.Program
}

func Compile(key, expression string) (*Program, error) {
	prog, err := expr.Compile(expression, expr.Env(Env{}), expr.AsAny())
	if err != nil {
		return nil, fmt.Errorf("compile extract[%s] %q: %w", key, expression, err)
	}
	return &Program{key: key, prog: prog}, nil
}

func (p *Program) Key() string { return p.key }

func (p *Program) Eval(env Env) (any, error) {
	out, err := expr.Run(p.prog, env)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// EvalAll runs a set of programs against one environment.
func EvalAll(env Env, programs []*Program) (map[string]any, error) {
	res := map[string]any{}
	var errs []string
	for _, p := range programs {
		v, err := p.Eval(env)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", p.key, err))
			continue
		}
		if v != nil {
			res[p.key] = v
		}
	}
	if len(errs) > 0 {
		return res, errors.New(strings.Join(errs, "; "))
	}
	return res, nil
}
