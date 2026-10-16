package openapi

import (
	"fmt"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// Validator checks JSON values against the spec's schemas — the subset of
// JSON Schema (2020-12, as OpenAPI 3.1 uses it) that openapi.yaml is written
// in. A keyword outside that subset is an error rather than ignored, so the
// spec never states a rule its tests do not check. Values are what
// encoding/json decodes into an any.
type Validator struct {
	doc map[string]any
}

func NewValidator(doc map[string]any) *Validator { return &Validator{doc: doc} }

// annotations carry no rule.
var annotations = map[string]bool{"description": true, "example": true, "default": true, "readOnly": true, "title": true}

var keywords = map[string]bool{
	"$ref": true, "type": true, "properties": true, "required": true, "additionalProperties": true,
	"items": true, "enum": true, "anyOf": true, "format": true, "pattern": true,
	"minLength": true, "maxLength": true, "minimum": true, "maximum": true, "exclusiveMinimum": true,
	"minItems": true, "maxItems": true,
}

// Resolve follows a local "#/a/b" reference.
func (v *Validator) Resolve(ref string) (map[string]any, error) {
	if !strings.HasPrefix(ref, "#/") {
		return nil, fmt.Errorf("only local references are supported: %s", ref)
	}
	var cur any = v.doc
	for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s: %q is not inside an object", ref, part)
		}
		if cur, ok = m[part]; !ok {
			return nil, fmt.Errorf("%s: no %q", ref, part)
		}
	}
	m, ok := cur.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s is not an object", ref)
	}
	return m, nil
}

// CheckSchema reports keywords outside the subset and references that do
// not resolve, anywhere inside schema.
func (v *Validator) CheckSchema(schema map[string]any) []string {
	var errs []string
	seen := map[string]bool{}
	var walk func(at string, s map[string]any)
	walk = func(at string, s map[string]any) {
		for k, val := range s {
			switch {
			case annotations[k]:
			case !keywords[k]:
				errs = append(errs, fmt.Sprintf("%s: unsupported keyword %q", at, k))
			case k == "$ref":
				ref, _ := val.(string)
				target, err := v.Resolve(ref)
				if err != nil {
					errs = append(errs, at+": "+err.Error())
				} else if !seen[ref] {
					seen[ref] = true
					walk(ref, target)
				}
			case k == "properties":
				props, _ := val.(map[string]any)
				for name, p := range props {
					if ps, ok := p.(map[string]any); ok {
						walk(at+"."+name, ps)
					} else {
						errs = append(errs, fmt.Sprintf("%s.%s: not a schema", at, name))
					}
				}
			case k == "items" || k == "additionalProperties":
				if sub, ok := val.(map[string]any); ok {
					walk(at+"/"+k, sub)
				}
			case k == "anyOf":
				list, _ := val.([]any)
				for i, alt := range list {
					if sub, ok := alt.(map[string]any); ok {
						walk(fmt.Sprintf("%s/anyOf/%d", at, i), sub)
					}
				}
			case k == "pattern":
				if _, err := regexp.Compile(fmt.Sprint(val)); err != nil {
					errs = append(errs, fmt.Sprintf("%s: pattern: %v", at, err))
				}
			case k == "format":
				switch val {
				case "date", "date-time", "uri":
				default:
					errs = append(errs, fmt.Sprintf("%s: unsupported format %v", at, val))
				}
			}
		}
	}
	walk("schema", schema)
	sort.Strings(errs)
	return errs
}

// Validate returns every way value breaks schema, each with its JSON path.
func (v *Validator) Validate(schema map[string]any, value any) []string {
	var errs []string
	v.validate("$", schema, value, &errs)
	return errs
}

func (v *Validator) validate(at string, s map[string]any, value any, errs *[]string) {
	fail := func(format string, args ...any) {
		*errs = append(*errs, at+": "+fmt.Sprintf(format, args...))
	}
	if ref, ok := s["$ref"].(string); ok {
		target, err := v.Resolve(ref)
		if err != nil {
			fail("%v", err)
			return
		}
		v.validate(at, target, value, errs)
	}
	if alts, ok := s["anyOf"].([]any); ok {
		matched := false
		for _, alt := range alts {
			sub, _ := alt.(map[string]any)
			var altErrs []string
			v.validate(at, sub, value, &altErrs)
			if len(altErrs) == 0 {
				matched = true
				break
			}
		}
		if !matched {
			fail("matches none of anyOf")
		}
	}
	if t, ok := s["type"]; ok && !typeMatches(t, value) {
		fail("want type %v, got %s", t, jsonType(value))
		return
	}
	if enum, ok := s["enum"].([]any); ok {
		found := false
		for _, e := range enum {
			if e == value {
				found = true
				break
			}
		}
		if !found {
			fail("%v is not one of %v", value, enum)
		}
	}
	switch x := value.(type) {
	case string:
		n := utf8.RuneCountInString(x)
		if min, ok := number(s["minLength"]); ok && float64(n) < min {
			fail("shorter than %v", min)
		}
		if max, ok := number(s["maxLength"]); ok && float64(n) > max {
			fail("longer than %v", max)
		}
		if p, ok := s["pattern"].(string); ok {
			if re, err := regexp.Compile(p); err == nil && !re.MatchString(x) {
				fail("%q does not match %s", x, p)
			}
		}
		switch s["format"] {
		case "date":
			if _, err := time.Parse(time.DateOnly, x); err != nil {
				fail("%q is not a date", x)
			}
		case "date-time":
			if _, err := time.Parse(time.RFC3339Nano, x); err != nil {
				fail("%q is not a date-time", x)
			}
		case "uri":
			if u, err := url.ParseRequestURI(x); err != nil || u.Scheme == "" {
				fail("%q is not an absolute URI", x)
			}
		}
	case float64:
		if min, ok := number(s["minimum"]); ok && x < min {
			fail("%v is below %v", x, min)
		}
		if max, ok := number(s["maximum"]); ok && x > max {
			fail("%v is above %v", x, max)
		}
		if min, ok := number(s["exclusiveMinimum"]); ok && x <= min {
			fail("%v is not above %v", x, min)
		}
	case []any:
		if min, ok := number(s["minItems"]); ok && float64(len(x)) < min {
			fail("fewer than %v items", min)
		}
		if max, ok := number(s["maxItems"]); ok && float64(len(x)) > max {
			fail("more than %v items", max)
		}
		if items, ok := s["items"].(map[string]any); ok {
			for i, item := range x {
				v.validate(fmt.Sprintf("%s[%d]", at, i), items, item, errs)
			}
		}
	case map[string]any:
		if req, ok := s["required"].([]any); ok {
			for _, name := range req {
				if _, ok := x[fmt.Sprint(name)]; !ok {
					fail("missing %v", name)
				}
			}
		}
		props, _ := s["properties"].(map[string]any)
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if p, ok := props[k].(map[string]any); ok {
				v.validate(at+"."+k, p, x[k], errs)
				continue
			}
			switch extra := s["additionalProperties"].(type) {
			case bool:
				if !extra {
					fail("unexpected property %q", k)
				}
			case map[string]any:
				v.validate(at+"."+k, extra, x[k], errs)
			}
		}
	}
}

func typeMatches(t any, value any) bool {
	switch tt := t.(type) {
	case string:
		return isType(tt, value)
	case []any:
		for _, one := range tt {
			if isType(fmt.Sprint(one), value) {
				return true
			}
		}
	}
	return false
}

func isType(t string, value any) bool {
	switch t {
	case "null":
		return value == nil
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "number":
		_, ok := value.(float64)
		return ok
	case "integer":
		f, ok := value.(float64)
		return ok && f == math.Trunc(f)
	case "array":
		_, ok := value.([]any)
		return ok
	case "object":
		_, ok := value.(map[string]any)
		return ok
	}
	return false
}

func jsonType(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case bool:
		return "boolean"
	case float64:
		return "number"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return fmt.Sprintf("%T", value)
}

func number(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case float64:
		return n, true
	}
	return 0, false
}
