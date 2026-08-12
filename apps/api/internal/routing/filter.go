package routing

import (
	"regexp"
	"strconv"
	"strings"
)

// norm matches lib/routing-simulate.ts's norm(): trim + lowercase, used by
// every string-comparison operator. Deliberately simple — no ISO
// code/locale normalization here. A filter authored with country "GB"
// must not silently match a request classified as "UK": if that ever
// matches, it means something upstream (classification) failed to
// canonicalize, and this package staying dumb-and-literal is what surfaces
// that bug instead of masking it (§58's "ISO code mismatch" case).
func norm(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// compareValues mirrors lib/routing-simulate.ts's compareValues: numeric
// comparison when both sides parse as numbers, lexicographic otherwise.
func compareValues(a, b string) int {
	na, errA := strconv.ParseFloat(a, 64)
	nb, errB := strconv.ParseFloat(b, 64)
	if strings.TrimSpace(a) != "" && strings.TrimSpace(b) != "" && errA == nil && errB == nil {
		switch {
		case na < nb:
			return -1
		case na > nb:
			return 1
		default:
			return 0
		}
	}
	return strings.Compare(a, b)
}

func (c FilterCondition) evaluate(attrs Attributes) Trace {
	requestValue := attrs[c.Field]
	value := c.Value

	var passed bool
	switch c.Operator {
	case OpIs:
		passed = norm(requestValue) == norm(value)
	case OpIsNot:
		passed = norm(requestValue) != norm(value)
	case OpIn:
		passed = containsNorm(strings.Split(value, ","), requestValue)
	case OpNotIn:
		passed = !containsNorm(strings.Split(value, ","), requestValue)
	case OpContains:
		passed = strings.Contains(norm(requestValue), norm(value))
	case OpNotContains:
		passed = !strings.Contains(norm(requestValue), norm(value))
	case OpStartsWith:
		passed = strings.HasPrefix(norm(requestValue), norm(value))
	case OpEndsWith:
		passed = strings.HasSuffix(norm(requestValue), norm(value))
	case OpMatches:
		re, err := regexp.Compile("(?i)" + value)
		passed = err == nil && re.MatchString(requestValue)
	case OpExists:
		passed = strings.TrimSpace(requestValue) != ""
	case OpNotExists:
		passed = strings.TrimSpace(requestValue) == ""
	case OpGT:
		passed = compareValues(requestValue, value) > 0
	case OpGTE:
		passed = compareValues(requestValue, value) >= 0
	case OpLT:
		passed = compareValues(requestValue, value) < 0
	case OpLTE:
		passed = compareValues(requestValue, value) <= 0
	case OpBetween:
		passed = compareValues(requestValue, c.Value) >= 0 && compareValues(requestValue, c.ValueTo) <= 0
	}

	return Trace{
		Kind:         TraceCondition,
		Passed:       passed,
		Field:        c.Field,
		Operator:     c.Operator,
		Value:        c.Value,
		ValueTo:      c.ValueTo,
		RequestValue: requestValue,
	}
}

func containsNorm(values []string, target string) bool {
	t := norm(target)
	for _, v := range values {
		if norm(v) == t {
			return true
		}
	}
	return false
}

func (g FilterGroup) evaluate(attrs Attributes) Trace {
	children := make([]Trace, len(g.Children))
	for i, child := range g.Children {
		children[i] = child.evaluate(attrs)
	}

	var passed bool
	switch {
	case len(g.Children) == 0:
		// Vacuous AND/OR — an empty group always passes, matching
		// lib/routing-simulate.ts exactly (a stream set with no filters
		// configured yet matches everything, not nothing).
		passed = true
	case g.Joiner == JoinAND:
		passed = true
		for _, c := range children {
			if !c.Passed {
				passed = false
				break
			}
		}
	default: // JoinOR
		passed = false
		for _, c := range children {
			if c.Passed {
				passed = true
				break
			}
		}
	}

	return Trace{
		Kind:     TraceGroup,
		Passed:   passed,
		Joiner:   g.Joiner,
		Children: children,
	}
}
