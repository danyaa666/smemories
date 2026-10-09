package api_test

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The contract lint (docs/api-contract.md section 7): every operation under /api/ in
// openapi.yaml must be self-documenting, or the build fails. /v1 and the probes are exempt
// until E10 moves them.

const apiPrefix = "/api/"

var (
	errorCodeRe     = regexp.MustCompile(`^ERROR_[A-Z0-9_]+$`)
	globalErrorCode = []string{"ERROR_INTERNAL", "ERROR_UNAUTHORIZED", "ERROR_PARAM"}
	authValues      = []string{"required", "public", "optional"}
	httpMethods     = []string{"get", "post", "put", "patch", "delete", "head", "options"}
)

func load(t *testing.T, file string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("%s: %v", file, err)
	}
	return doc
}

func obj(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func text(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// lintContract returns one line per violation, each starting with "METHOD /path: ".
func lintContract(doc map[string]any) []string {
	var out []string
	paths := obj(doc["paths"])
	for _, path := range sortedKeys(paths) {
		if !strings.HasPrefix(path, apiPrefix) {
			continue
		}
		item := obj(paths[path])
		for _, method := range httpMethods {
			if op := obj(item[method]); op != nil {
				l := &opLint{doc: doc, who: strings.ToUpper(method) + " " + path, path: path, op: op, seen: map[string]bool{}}
				l.run()
				out = append(out, l.out...)
			}
		}
	}
	return out
}

type opLint struct {
	doc  map[string]any
	who  string
	path string
	op   map[string]any
	seen map[string]bool // $refs already walked for this operation
	out  []string
}

func (l *opLint) bad(format string, args ...any) {
	l.out = append(l.out, l.who+": "+fmt.Sprintf(format, args...))
}

func (l *opLint) run() {
	for _, f := range []string{"summary", "description", "operationId"} {
		if text(l.op[f]) == "" {
			l.bad("missing %s", f)
		}
	}
	if auth := text(l.op["x-auth"]); !slices.Contains(authValues, auth) {
		l.bad("x-auth must be one of %v, got %q", authValues, auth)
	}
	l.errorCodes()

	if body := obj(l.op["requestBody"]); body != nil {
		for _, mt := range obj(body["content"]) {
			l.schema("request", obj(obj(mt)["schema"]))
		}
	}
	for _, status := range sortedKeys(obj(l.op["responses"])) {
		for _, mt := range obj(obj(obj(l.op["responses"])[status])["content"]) {
			l.schema("response "+status, obj(obj(mt)["schema"]))
		}
	}
	l.example200()
	if strings.HasSuffix(l.path, "-list") {
		l.listShape()
	}
}

func (l *opLint) errorCodes() {
	raw, ok := l.op["x-error-codes"]
	if !ok {
		l.bad("missing x-error-codes (use an empty list when there are none)")
		return
	}
	list, isList := raw.([]any)
	if !isList && raw != nil {
		l.bad("x-error-codes must be a list")
		return
	}
	for _, e := range list {
		code, desc := text(obj(e)["code"]), text(obj(e)["description"])
		switch {
		case !errorCodeRe.MatchString(code):
			l.bad("x-error-codes: %q does not match ^ERROR_[A-Z0-9_]+$", code)
		case slices.Contains(globalErrorCode, code):
			l.bad("x-error-codes: %s is global and must not be listed", code)
		}
		if desc == "" {
			l.bad("x-error-codes: %q has no description", code)
		}
	}
}

func (l *opLint) example200() {
	resp := obj(obj(l.op["responses"])["200"])
	if resp == nil {
		l.bad("missing 200 response")
		return
	}
	for _, mt := range obj(resp["content"]) {
		if ex := obj(obj(mt)["examples"]); len(ex) > 0 {
			return
		}
	}
	l.bad("200 response has no examples entry")
}

// listShape: a list endpoint (path ends in -list) answers data: {items, next_id}.
func (l *opLint) listShape() {
	for _, mt := range obj(obj(obj(l.op["responses"])["200"])["content"]) {
		s := l.resolve(obj(obj(mt)["schema"]))
		if data := obj(obj(s["properties"])["data"]); data != nil {
			s = l.resolve(data)
		}
		props := obj(s["properties"])
		if props["items"] == nil || props["next_id"] == nil {
			l.bad("list response must have items and next_id in data")
		}
		return
	}
	l.bad("list endpoint has no 200 response schema")
}

func (l *opLint) resolve(s map[string]any) map[string]any {
	ref := text(s["$ref"])
	if ref == "" {
		return s
	}
	var cur any = l.doc
	for _, seg := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		cur = obj(cur)[seg]
	}
	if t := obj(cur); t != nil {
		return t
	}
	l.bad("unresolvable $ref %s", ref)
	return map[string]any{}
}

// schema checks every property below s, following $ref once per operation.
func (l *opLint) schema(where string, s map[string]any) {
	if s == nil {
		return
	}
	if ref := text(s["$ref"]); ref != "" {
		if l.seen[ref] {
			return
		}
		l.seen[ref] = true
		s = l.resolve(s)
	}
	props := obj(s["properties"])
	for _, name := range sortedKeys(props) {
		p := obj(props[name])
		desc := text(p["description"])
		if desc == "" {
			desc = text(l.resolve(p)["description"]) // a $ref may carry the description in its target
		}
		if desc == "" {
			l.bad("%s: property %q has no description", where, name)
		}
		if strings.HasSuffix(name, "_at") {
			l.timestamp(where, name, l.resolve(p), desc)
		}
		l.schema(where, p)
	}
	l.schema(where, obj(s["items"]))
	l.schema(where, obj(s["additionalProperties"]))
	for _, k := range []string{"allOf", "oneOf", "anyOf"} {
		if list, ok := s[k].([]any); ok {
			for _, sub := range list {
				l.schema(where, obj(sub))
			}
		}
	}
}

func (l *opLint) timestamp(where, name string, p map[string]any, desc string) {
	types := []string{}
	switch v := p["type"].(type) {
	case string:
		types = append(types, v)
	case []any:
		for _, x := range v {
			types = append(types, text(x))
		}
	}
	if !slices.Contains(types, "integer") || text(p["format"]) != "int64" {
		l.bad("%s: timestamp %q must be integer/int64", where, name)
	}
	if !strings.Contains(desc, "Unix ms") {
		l.bad("%s: timestamp %q description must say \"Unix ms\"", where, name)
	}
}

func TestOpenAPIContract(t *testing.T) {
	if v := lintContract(load(t, "openapi.yaml")); len(v) > 0 {
		t.Errorf("api/openapi.yaml breaks docs/api-contract.md section 7:\n  %s", strings.Join(v, "\n  "))
	}
}

// The linter itself: the good fixture passes, and every rule has an operation in the bad
// fixture that must be reported for exactly that rule.
func TestContractLintFixtures(t *testing.T) {
	if v := lintContract(load(t, "testdata/contract_good.yaml")); len(v) > 0 {
		t.Errorf("good fixture reported:\n  %s", strings.Join(v, "\n  "))
	}

	bad := lintContract(load(t, "testdata/contract_bad.yaml"))
	for _, c := range []struct{ who, want string }{
		{"POST /api/bad/no-summary", "missing summary"},
		{"POST /api/bad/no-description", "missing description"},
		{"POST /api/bad/no-operation-id", "missing operationId"},
		{"POST /api/bad/bad-auth", "x-auth must be one of"},
		{"POST /api/bad/no-error-codes", "missing x-error-codes"},
		{"POST /api/bad/bad-error-code", "does not match"},
		{"POST /api/bad/global-error-code", "ERROR_PARAM is global"},
		{"POST /api/bad/error-code-no-description", "has no description"},
		{"POST /api/bad/no-property-description", `property "title" has no description`},
		{"POST /api/bad/no-ref-property-description", `property "name" has no description`},
		{"POST /api/bad/no-example", "200 response has no examples entry"},
		{"POST /api/bad/no-200", "missing 200 response"},
		{"POST /api/bad/timestamp-type", `timestamp "created_at" must be integer/int64`},
		{"POST /api/bad/timestamp-unit", `description must say "Unix ms"`},
		{"GET /api/bad/get-list", "list response must have items and next_id"},
	} {
		if !slices.ContainsFunc(bad, func(v string) bool { return strings.HasPrefix(v, c.who+": ") && strings.Contains(v, c.want) }) {
			t.Errorf("%s: want a violation containing %q, got:\n  %s", c.who, c.want, strings.Join(bad, "\n  "))
		}
	}
	for _, v := range bad {
		if strings.Contains(v, "/v1/") || strings.Contains(v, "/healthz") {
			t.Errorf("only /api/ operations are linted, got %q", v)
		}
	}
}
