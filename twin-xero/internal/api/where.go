package api

import (
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

// cmpExpr is one comparison of a field with a literal. The literal is a string,
// a float64, a bool, nil, a time.Time (DateTime) or a guid.
type cmpExpr struct {
	path, op string
	lit      any
}

// parseComparison reads one comparison of Xero's C#-like where language (quirk
// query-001): a field path, one of ==, !=, <, <=, > or >=, and a literal. Literals
// are "strings", numbers, true, false, null, DateTime(y, m, d[, h, m, s]) and
// Guid("..."). A double-encoded value (%2522) reaches the parser still encoded
// and is a syntax error, as in Xero.
func parseComparison(src string) (cmpExpr, error) {
	p := &whereParser{src: src}
	c, err := p.comparison()
	if err != nil {
		return cmpExpr{}, err
	}
	if p.skip(); p.pos < len(p.src) {
		return cmpExpr{}, werr("unexpected %q at position %d", p.src[p.pos:p.pos+1], p.pos)
	}
	return c, nil
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

func (p *whereParser) comparison() (cmpExpr, error) {
	path := p.path()
	if path == "" {
		return cmpExpr{}, werr("expected a field name at position %d", p.pos)
	}
	op := p.accept("==", "!=", "<=", ">=", "<", ">")
	if op == "" {
		return cmpExpr{}, werr("expected a comparison after %s at position %d", path, p.pos)
	}
	lit, err := p.literal()
	return cmpExpr{path, op, lit}, err
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
