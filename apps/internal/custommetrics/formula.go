package custommetrics

import (
	"fmt"
	"strings"
	"unicode"
)

// FormulaError mirrors apps/web/src/lib/formula-engine.ts's FormulaError
// — same message text where practical, so a formula the frontend's own
// live validator already accepted never gets a surprising, differently-
// worded rejection here.
type FormulaError struct {
	Message string
	Pos     int
}

func (e *FormulaError) Error() string { return e.Message }

type tokenType int

const (
	tokNumber tokenType = iota
	tokMetric
	tokIdent
	tokLParen
	tokRParen
	tokComma
	tokPlus
	tokMinus
	tokStar
	tokSlash
	tokGT
	tokLT
	tokGE
	tokLE
	tokEQ
	tokNE
	tokEOF
)

type token struct {
	typ tokenType
	val string
	pos int
}

func tokenize(input string) ([]token, error) {
	var tokens []token
	runes := []rune(input)
	i := 0
	for i < len(runes) {
		ch := runes[i]
		if unicode.IsSpace(ch) {
			i++
			continue
		}
		if ch == '{' {
			end := strings.IndexRune(string(runes[i:]), '}')
			if end == -1 {
				return nil, &FormulaError{"Unterminated metric token — missing closing }", i}
			}
			end += i
			name := strings.TrimSpace(string(runes[i+1 : end]))
			if name == "" {
				return nil, &FormulaError{"Empty metric token {}", i}
			}
			tokens = append(tokens, token{tokMetric, name, i})
			i = end + 1
			continue
		}
		if unicode.IsDigit(ch) || ch == '.' {
			j := i
			for j < len(runes) && (unicode.IsDigit(runes[j]) || runes[j] == '.') {
				j++
			}
			raw := string(runes[i:j])
			if !isValidNumber(raw) {
				return nil, &FormulaError{fmt.Sprintf("Invalid number %q", raw), i}
			}
			tokens = append(tokens, token{tokNumber, raw, i})
			i = j
			continue
		}
		if unicode.IsLetter(ch) || ch == '_' {
			j := i
			for j < len(runes) && (unicode.IsLetter(runes[j]) || unicode.IsDigit(runes[j]) || runes[j] == '_') {
				j++
			}
			tokens = append(tokens, token{tokIdent, string(runes[i:j]), i})
			i = j
			continue
		}
		switch ch {
		case '(':
			tokens = append(tokens, token{tokLParen, "(", i})
			i++
			continue
		case ')':
			tokens = append(tokens, token{tokRParen, ")", i})
			i++
			continue
		case ',':
			tokens = append(tokens, token{tokComma, ",", i})
			i++
			continue
		case '+':
			tokens = append(tokens, token{tokPlus, "+", i})
			i++
			continue
		case '-':
			tokens = append(tokens, token{tokMinus, "-", i})
			i++
			continue
		case '*', '×':
			tokens = append(tokens, token{tokStar, string(ch), i})
			i++
			continue
		case '/', '÷':
			tokens = append(tokens, token{tokSlash, string(ch), i})
			i++
			continue
		case '>':
			if i+1 < len(runes) && runes[i+1] == '=' {
				tokens = append(tokens, token{tokGE, ">=", i})
				i += 2
			} else {
				tokens = append(tokens, token{tokGT, ">", i})
				i++
			}
			continue
		case '<':
			if i+1 < len(runes) && runes[i+1] == '=' {
				tokens = append(tokens, token{tokLE, "<=", i})
				i += 2
			} else {
				tokens = append(tokens, token{tokLT, "<", i})
				i++
			}
			continue
		case '=':
			if i+1 < len(runes) && runes[i+1] == '=' {
				tokens = append(tokens, token{tokEQ, "==", i})
				i += 2
				continue
			}
		case '!':
			if i+1 < len(runes) && runes[i+1] == '=' {
				tokens = append(tokens, token{tokNE, "!=", i})
				i += 2
				continue
			}
		}
		return nil, &FormulaError{fmt.Sprintf("Unexpected character %q", string(ch)), i}
	}
	tokens = append(tokens, token{tokEOF, "", len(runes)})
	return tokens, nil
}

func isValidNumber(s string) bool {
	dotSeen := false
	digitSeen := false
	for _, c := range s {
		if c == '.' {
			if dotSeen {
				return false
			}
			dotSeen = true
			continue
		}
		if !unicode.IsDigit(c) {
			return false
		}
		digitSeen = true
	}
	return digitSeen
}

type functionSpec struct {
	min, max int // max = -1 means unbounded
}

var functionSignatures = map[string]functionSpec{
	"DIV":     {2, 2},
	"EMPTYIF": {2, 2},
	"IF":      {3, 3},
	"ROUND":   {1, 2},
	"ABS":     {1, 1},
	"MIN":     {1, -1},
	"MAX":     {1, -1},
}

type nodeKind int

const (
	nodeNumber nodeKind = iota
	nodeMetric
	nodeUnary
	nodeBinary
	nodeCall
)

type node struct {
	kind  nodeKind
	value float64 // number
	id    string  // metric
	op    string  // binary op
	arg   *node   // unary
	left  *node   // binary
	right *node   // binary
	name  string  // call
	args  []*node // call
}

type parser struct {
	tokens []token
	pos    int
}

func (p *parser) peek() token { return p.tokens[p.pos] }
func (p *parser) next() token {
	t := p.tokens[p.pos]
	p.pos++
	return t
}
func (p *parser) expect(typ tokenType, label string) (token, error) {
	t := p.next()
	if t.typ != typ {
		return t, &FormulaError{"Expected " + label, t.pos}
	}
	return t, nil
}

func parseFormula(input string) (*node, error) {
	tokens, err := tokenize(input)
	if err != nil {
		return nil, err
	}
	p := &parser{tokens: tokens}
	n, err := p.parseComparison()
	if err != nil {
		return nil, err
	}
	if p.peek().typ != tokEOF {
		return nil, &FormulaError{fmt.Sprintf("Unexpected %q", p.peek().val), p.peek().pos}
	}
	return n, nil
}

func (p *parser) parseComparison() (*node, error) {
	left, err := p.parseAdd()
	if err != nil {
		return nil, err
	}
	compareOps := map[tokenType]string{tokGT: ">", tokLT: "<", tokGE: ">=", tokLE: "<=", tokEQ: "==", tokNE: "!="}
	if op, ok := compareOps[p.peek().typ]; ok {
		p.next()
		right, err := p.parseAdd()
		if err != nil {
			return nil, err
		}
		return &node{kind: nodeBinary, op: op, left: left, right: right}, nil
	}
	return left, nil
}

func (p *parser) parseAdd() (*node, error) {
	left, err := p.parseMul()
	if err != nil {
		return nil, err
	}
	for p.peek().typ == tokPlus || p.peek().typ == tokMinus {
		op := "+"
		if p.next().typ == tokMinus {
			op = "-"
		}
		right, err := p.parseMul()
		if err != nil {
			return nil, err
		}
		left = &node{kind: nodeBinary, op: op, left: left, right: right}
	}
	return left, nil
}

func (p *parser) parseMul() (*node, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.peek().typ == tokStar || p.peek().typ == tokSlash {
		op := "*"
		if p.next().typ == tokSlash {
			op = "/"
		}
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = &node{kind: nodeBinary, op: op, left: left, right: right}
	}
	return left, nil
}

func (p *parser) parseUnary() (*node, error) {
	if p.peek().typ == tokMinus {
		p.next()
		arg, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &node{kind: nodeUnary, arg: arg}, nil
	}
	return p.parseAtom()
}

func (p *parser) parseAtom() (*node, error) {
	t := p.peek()
	switch t.typ {
	case tokNumber:
		p.next()
		var v float64
		fmt.Sscanf(t.val, "%f", &v)
		return &node{kind: nodeNumber, value: v}, nil
	case tokMetric:
		p.next()
		return &node{kind: nodeMetric, id: t.val}, nil
	case tokLParen:
		p.next()
		inner, err := p.parseComparison()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tokRParen, "')'"); err != nil {
			return nil, err
		}
		return inner, nil
	case tokIdent:
		p.next()
		name := strings.ToUpper(t.val)
		spec, ok := functionSignatures[name]
		if !ok {
			return nil, &FormulaError{fmt.Sprintf("Unknown function %q", t.val), t.pos}
		}
		if _, err := p.expect(tokLParen, "'(' after "+name); err != nil {
			return nil, err
		}
		var args []*node
		if p.peek().typ != tokRParen {
			arg, err := p.parseComparison()
			if err != nil {
				return nil, err
			}
			args = append(args, arg)
			for p.peek().typ == tokComma {
				p.next()
				arg, err := p.parseComparison()
				if err != nil {
					return nil, err
				}
				args = append(args, arg)
			}
		}
		if _, err := p.expect(tokRParen, "')'"); err != nil {
			return nil, err
		}
		if len(args) < spec.min || (spec.max != -1 && len(args) > spec.max) {
			arity := fmt.Sprintf("%d", spec.min)
			if spec.max != spec.min {
				if spec.max == -1 {
					arity = fmt.Sprintf("%d-many", spec.min)
				} else {
					arity = fmt.Sprintf("%d-%d", spec.min, spec.max)
				}
			}
			return nil, &FormulaError{fmt.Sprintf("%s expects %s argument(s), got %d", name, arity, len(args)), t.pos}
		}
		return &node{kind: nodeCall, name: name, args: args}, nil
	case tokEOF:
		return nil, &FormulaError{"Formula ends unexpectedly — missing an operand", t.pos}
	}
	return nil, &FormulaError{fmt.Sprintf("Unexpected %q", t.val), t.pos}
}

func collectMetricIDs(n *node, out map[string]bool) {
	if n == nil {
		return
	}
	switch n.kind {
	case nodeMetric:
		out[n.id] = true
	case nodeUnary:
		collectMetricIDs(n.arg, out)
	case nodeBinary:
		collectMetricIDs(n.left, out)
		collectMetricIDs(n.right, out)
	case nodeCall:
		for _, a := range n.args {
			collectMetricIDs(a, out)
		}
	}
}

// FormulaValidation mirrors the frontend's FormulaValidation exactly —
// same three constraints (unknown metric, LTV-forbidden via
// !insertable, single data source), so a formula that validates on one
// side always validates on the other.
type FormulaValidation struct {
	Valid         bool
	Error         string
	UsedMetricIDs []string
	DataSource    string
}

func validateFormula(formula string) FormulaValidation {
	trimmed := strings.TrimSpace(formula)
	if trimmed == "" {
		return FormulaValidation{Error: "Formula is empty"}
	}

	ast, err := parseFormula(trimmed)
	if err != nil {
		return FormulaValidation{Error: err.Error()}
	}

	idsSet := map[string]bool{}
	collectMetricIDs(ast, idsSet)
	if len(idsSet) == 0 {
		return FormulaValidation{Error: "Formula must reference at least one metric"}
	}
	usedMetricIDs := make([]string, 0, len(idsSet))
	for id := range idsSet {
		usedMetricIDs = append(usedMetricIDs, id)
	}

	dataSources := map[dataSource]bool{}
	for _, id := range usedMetricIDs {
		meta, ok := metricCatalog[id]
		if !ok {
			return FormulaValidation{Error: fmt.Sprintf("Unknown metric {%s}", id)}
		}
		if !meta.insertable {
			return FormulaValidation{Error: fmt.Sprintf("metric %q can't be used in custom metric formulas", id)}
		}
		dataSources[meta.dataSource] = true
	}
	if len(dataSources) > 1 {
		names := make([]string, 0, len(dataSources))
		for ds := range dataSources {
			names = append(names, string(ds))
		}
		return FormulaValidation{Error: "A formula can only draw from one data source — this mixes " + strings.Join(names, " and ")}
	}

	var ds dataSource
	for k := range dataSources {
		ds = k
	}
	return FormulaValidation{Valid: true, UsedMetricIDs: usedMetricIDs, DataSource: string(ds)}
}
