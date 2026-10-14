package evaluate

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Eval 简单公式求值，支持: x + - * / 和括号
func Eval(formula string, x float64) (float64, error) {
	if formula == "" {
		return x, nil
	}
	expr := strings.ReplaceAll(formula, "x", strconv.FormatFloat(x, 'g', -1, 64))
	result, err := parse(expr)
	if err != nil {
		return 0, fmt.Errorf("eval '%s': %w", formula, err)
	}
	return result, nil
}

type parser struct {
	s   string
	pos int
}

func parse(expr string) (float64, error) {
	p := &parser{s: expr}
	v, err := p.parseExpr()
	if err != nil {
		return 0, err
	}
	p.skipSpace()
	if p.pos < len(p.s) {
		return 0, fmt.Errorf("unexpected char '%c' at %d", p.s[p.pos], p.pos)
	}
	return v, nil
}

func (p *parser) parseExpr() (float64, error) {
	left, err := p.parseTerm()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpace()
		if p.pos >= len(p.s) {
			return left, nil
		}
		ch := p.s[p.pos]
		if ch != '+' && ch != '-' {
			return left, nil
		}
		p.pos++
		right, err := p.parseTerm()
		if err != nil {
			return 0, err
		}
		if ch == '+' {
			left += right
		} else {
			left -= right
		}
	}
}

func (p *parser) parseTerm() (float64, error) {
	left, err := p.parseFactor()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpace()
		if p.pos >= len(p.s) {
			return left, nil
		}
		ch := p.s[p.pos]
		if ch != '*' && ch != '/' {
			return left, nil
		}
		p.pos++
		right, err := p.parseFactor()
		if err != nil {
			return 0, err
		}
		if ch == '*' {
			left *= right
		} else {
			if right == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			left /= right
		}
	}
}

func (p *parser) parseFactor() (float64, error) {
	p.skipSpace()
	if p.pos >= len(p.s) {
		return 0, fmt.Errorf("unexpected end")
	}
	ch := p.s[p.pos]
	if ch == '(' {
		p.pos++
		v, err := p.parseExpr()
		if err != nil {
			return 0, err
		}
		p.skipSpace()
		if p.pos >= len(p.s) || p.s[p.pos] != ')' {
			return 0, fmt.Errorf("missing closing paren at %d", p.pos)
		}
		p.pos++
		return v, nil
	}
	if ch == '-' {
		p.pos++
		v, err := p.parseFactor()
		if err != nil {
			return 0, err
		}
		return -v, nil
	}
	if ch == '+' {
		p.pos++
		return p.parseFactor()
	}
	if ch >= '0' && ch <= '9' || ch == '.' {
		return p.parseNumber()
	}
	return 0, fmt.Errorf("unexpected '%c' at %d [expr=%s]", ch, p.pos, p.s)
}

func (p *parser) parseNumber() (float64, error) {
	start := p.pos
	for p.pos < len(p.s) && (isDigit(p.s[p.pos]) || p.s[p.pos] == '.') {
		p.pos++
	}
	n, err := strconv.ParseFloat(p.s[start:p.pos], 64)
	if err != nil {
		return 0, err
	}
	if math.IsInf(n, 1) || math.IsInf(n, -1) {
		return 0, fmt.Errorf("overflow")
	}
	return n, nil
}

func (p *parser) skipSpace() {
	for p.pos < len(p.s) && p.s[p.pos] == ' ' {
		p.pos++
	}
}

func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}
