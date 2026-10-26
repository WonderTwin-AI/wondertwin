package api

import (
	"cmp"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// WhereError is a where expression that cannot be read or applied.
type WhereError struct{ Msg string }

func (e *WhereError) Error() string { return e.Msg }

func werr(format string, a ...any) error { return &WhereError{Msg: fmt.Sprintf(format, a...)} }

// guid is a Guid("...") literal; it compares without regard to case.
type guid string

type (
	// cmpExpr is one comparison of a field with a literal. The literal is a
	// string, a float64, a bool, nil, a time.Time (DateTime) or a guid.
	cmpExpr struct {
		path, op string
		lit      any
	}
	// callExpr is a string call on a field: path.Contains("arg").
	callExpr struct{ path, method, arg string }
	orExpr   struct{ l, r Where }
	andExpr  struct{ l, r Where }
)

// Getter reads a field by dotted path (Contact.ContactID) from a record. A value
// is a string, a number (float64, int or int64), a bool, a time.Time or nil; ok
// is false for a path the record does not have, which reads as null.
type Getter func(path string) (value any, ok bool)

// Where is a parsed where expression.
type Where interface {
	// Match reports whether a record satisfies the expression. A WhereError
	// means the expression compares a field with a literal of another type.
	Match(get Getter) (bool, error)
	// String renders the expression fully parenthesised, so a test can see how
	// it was grouped.
	String() string
}

// ParseWhere reads Xero's C#-like where language (quirk query-001): comparisons
// of a field path with a literal (==, !=, <, <=, > and >=), the string calls
// .Contains, .StartsWith and .EndsWith, parentheses, and expressions joined by
// and, or, AND, OR, && or ||, where and binds tighter than or. Literals are
// "strings", numbers, true, false, null, DateTime(y, m, d[, h, m, s]) and
// Guid("..."). A double-encoded value (%2522) reaches the parser still encoded
// and is a syntax error, as in Xero.
func ParseWhere(src string) (Where, error) {
	p := &whereParser{src: src}
	e, err := p.or()
	if err != nil {
		return nil, err
	}
	if p.skip(); p.pos < len(p.src) {
		return nil, werr("unexpected %q at position %d", p.src[p.pos:p.pos+1], p.pos)
	}
	return e, nil
}

type whereParser struct {
	src string
	pos int
}

func (p *whereParser) skip() {
	for p.pos < len(p.src) && strings.ContainsRune(" \t\r\n", rune(p.src[p.pos])) {
		p.pos++
	}
}

func isIdent(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// accept consumes the first of words that comes next. A word made of letters
// must end where an identifier ends, so "null" does not match the start of
// "nullable".
func (p *whereParser) accept(words ...string) string {
	p.skip()
	for _, w := range words {
		if !strings.HasPrefix(p.src[p.pos:], w) {
			continue
		}
		end := p.pos + len(w)
		if isIdent(w[0]) && end < len(p.src) && isIdent(p.src[end]) {
			continue
		}
		p.pos = end
		return w
	}
	return ""
}

func (p *whereParser) or() (Where, error) {
	l, err := p.and()
	for err == nil && p.accept("||", "or", "OR") != "" {
		var r Where
		if r, err = p.and(); err == nil {
			l = orExpr{l, r}
		}
	}
	return l, err
}

func (p *whereParser) and() (Where, error) {
	l, err := p.term()
	for err == nil && p.accept("&&", "and", "AND") != "" {
		var r Where
		if r, err = p.term(); err == nil {
			l = andExpr{l, r}
		}
	}
	return l, err
}

// term is a parenthesised expression, a string call or a comparison.
func (p *whereParser) term() (Where, error) {
	if p.accept("(") != "" {
		e, err := p.or()
		if err == nil && p.accept(")") == "" {
			err = werr("missing ) at position %d", p.pos)
		}
		return e, err
	}
	path := p.path()
	if path == "" {
		return nil, werr("expected a field name at position %d", p.pos)
	}
	if i := strings.LastIndexByte(path, '.'); i > 0 && p.accept("(") != "" {
		method := path[i+1:]
		if method != "Contains" && method != "StartsWith" && method != "EndsWith" {
			return nil, werr("unknown method %s", method)
		}
		arg, err := p.callArg()
		return callExpr{path[:i], method, arg}, err
	}
	op := p.accept("==", "!=", "<=", ">=", "<", ">", "=")
	if op == "" {
		return nil, werr("expected a comparison after %s at position %d", path, p.pos)
	}
	if op == "=" { // localstack/finance-ops sends AccountNumber="..." against Xero
		op = "=="
	}
	lit, err := p.literal()
	return cmpExpr{path, op, lit}, err
}

// callArg reads the single string argument of a call, after its (.
func (p *whereParser) callArg() (string, error) {
	p.skip()
	if p.pos >= len(p.src) || p.src[p.pos] != '"' {
		return "", werr("a string call takes one string, at position %d", p.pos)
	}
	s, err := p.str()
	if err == nil && p.accept(")") == "" {
		err = werr("missing ) at position %d", p.pos)
	}
	return s, err
}

func (p *whereParser) path() string {
	p.skip()
	start := p.pos
	for p.pos < len(p.src) && (isIdent(p.src[p.pos]) || p.src[p.pos] == '.') {
		p.pos++
	}
	return p.src[start:p.pos]
}

func (p *whereParser) literal() (any, error) {
	switch {
	case p.accept("true") != "":
		return true, nil
	case p.accept("false") != "":
		return false, nil
	case p.accept("null") != "":
		return nil, nil
	case p.accept("DateTime") != "":
		return p.dateTime()
	case p.accept("Guid") != "":
		s, err := p.parenString()
		return guid(s), err
	}
	p.skip()
	if p.pos < len(p.src) && p.src[p.pos] == '"' {
		return p.str()
	}
	start := p.pos
	for p.pos < len(p.src) && strings.ContainsRune("+-.0123456789", rune(p.src[p.pos])) {
		p.pos++
	}
	n, err := strconv.ParseFloat(p.src[start:p.pos], 64)
	if err != nil {
		return nil, werr("expected a value at position %d", start)
	}
	return n, nil
}

func (p *whereParser) str() (string, error) {
	end := strings.IndexByte(p.src[p.pos+1:], '"')
	if end < 0 {
		return "", werr("unterminated string at position %d", p.pos)
	}
	s := p.src[p.pos+1 : p.pos+1+end]
	p.pos += end + 2
	return s, nil
}

func (p *whereParser) parenString() (string, error) {
	if p.accept("(") == "" {
		return "", werr("expected ( at position %d", p.pos)
	}
	p.skip()
	if p.pos >= len(p.src) || p.src[p.pos] != '"' {
		return "", werr("expected a string at position %d", p.pos)
	}
	s, err := p.str()
	if err == nil && p.accept(")") == "" {
		err = werr("missing ) at position %d", p.pos)
	}
	return s, err
}

// dateTime reads (y, m, d) or (y, m, d, h, m, s), in UTC.
func (p *whereParser) dateTime() (any, error) {
	if p.accept("(") == "" {
		return nil, werr("expected ( after DateTime at position %d", p.pos)
	}
	var part []int
	for {
		p.skip()
		start := p.pos
		for p.pos < len(p.src) && p.src[p.pos] >= '0' && p.src[p.pos] <= '9' {
			p.pos++
		}
		v, err := strconv.Atoi(p.src[start:p.pos])
		if err != nil {
			return nil, werr("DateTime needs numbers at position %d", start)
		}
		part = append(part, v)
		if p.accept(",") == "" {
			break
		}
	}
	if p.accept(")") == "" || (len(part) != 3 && len(part) != 6) {
		return nil, werr("DateTime takes year, month, day and optionally hour, minute, second")
	}
	part = append(part, 0, 0, 0)
	t := time.Date(part[0], time.Month(part[1]), part[2], part[3], part[4], part[5], 0, time.UTC)
	y, mo, d := t.Date()
	if y != part[0] || int(mo) != part[1] || d != part[2] || t.Hour() != part[3] || t.Minute() != part[4] || t.Second() != part[5] {
		return nil, werr("DateTime(%d,%d,%d,%d,%d,%d) is not a date", part[0], part[1], part[2], part[3], part[4], part[5])
	}
	return t, nil
}

func (e orExpr) String() string  { return "(" + e.l.String() + " or " + e.r.String() + ")" }
func (e andExpr) String() string { return "(" + e.l.String() + " and " + e.r.String() + ")" }
func (e callExpr) String() string {
	return fmt.Sprintf("%s.%s(%q)", e.path, e.method, e.arg)
}
func (e cmpExpr) String() string { return e.path + e.op + litString(e.lit) }

func litString(lit any) string {
	switch l := lit.(type) {
	case nil:
		return "null"
	case string:
		return strconv.Quote(l)
	case guid:
		return fmt.Sprintf("Guid(%q)", string(l))
	case time.Time:
		return fmt.Sprintf("DateTime(%d,%d,%d,%d,%d,%d)", l.Year(), l.Month(), l.Day(), l.Hour(), l.Minute(), l.Second())
	}
	return fmt.Sprint(lit)
}

func (e orExpr) Match(g Getter) (bool, error) {
	if ok, err := e.l.Match(g); ok || err != nil {
		return ok, err
	}
	return e.r.Match(g)
}

func (e andExpr) Match(g Getter) (bool, error) {
	if ok, err := e.l.Match(g); !ok || err != nil {
		return false, err
	}
	return e.r.Match(g)
}

// Match applies a string call. A field that is not a string never matches.
func (e callExpr) Match(g Getter) (bool, error) {
	v, _ := g(e.path)
	s, ok := v.(string)
	switch {
	case !ok:
		return false, nil
	case e.method == "Contains":
		return strings.Contains(s, e.arg), nil
	case e.method == "StartsWith":
		return strings.HasPrefix(s, e.arg), nil
	}
	return strings.HasSuffix(s, e.arg), nil
}

func (e cmpExpr) Match(g Getter) (bool, error) {
	if _, isBool := e.lit.(bool); isBool && e.op != "==" && e.op != "!=" {
		return false, werr("cannot compare %s: booleans only support == and !=", e.path)
	}
	v, _ := g(e.path)
	c, err := compare(v, e.lit)
	if err != nil {
		return false, werr("cannot compare %s: %v", e.path, err)
	}
	switch e.op {
	case "==":
		return c == 0, nil
	case "!=":
		return c != 0, nil
	case "<":
		return c == -1, nil
	case "<=":
		return c == -1 || c == 0, nil
	case ">":
		return c == 1, nil
	}
	return c == 1 || c == 0, nil // >=
}

// unordered is compare's answer when exactly one side is null: such values are
// neither equal nor ordered, so only != holds.
const unordered = 2

// compare orders a record value against a literal: -1, 0 or 1, or unordered. A
// number is compared as a float, a Guid without regard to case, a string
// byte for byte, and a bool for equality only (Match refuses to order one).
func compare(v, lit any) (int, error) {
	if v == nil || lit == nil {
		if v == nil && lit == nil {
			return 0, nil
		}
		return unordered, nil
	}
	switch l := lit.(type) {
	case guid:
		if s, ok := v.(string); ok {
			return strings.Compare(strings.ToLower(s), strings.ToLower(string(l))), nil
		}
	case string:
		if s, ok := v.(string); ok {
			return strings.Compare(s, l), nil
		}
	case bool:
		if b, ok := v.(bool); ok {
			if b == l {
				return 0, nil
			}
			return 1, nil // unequal; a bool has no order
		}
	case time.Time:
		if t, ok := v.(time.Time); ok {
			return t.Compare(l), nil
		}
	case float64:
		if f, ok := toFloat(v); ok {
			return cmp.Compare(f, l), nil
		}
	}
	return 0, fmt.Errorf("%T against %T", v, lit)
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}
