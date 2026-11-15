// Package calc evaluates arithmetic expressions and solves one-variable
// equations using only the Go standard library.
package calc

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

type kind int

const (
	tEOF kind = iota
	tNum
	tIdent
	tOp
	tLParen
	tRParen
	tComma
)

type token struct {
	k   kind
	s   string
	v   float64
	pos int
}

var replacer = strings.NewReplacer(
	"**", "^",
	"×", "*", "·", "*", "⋅", "*", "∗", "*",
	"÷", "/", "∕", "/",
	"−", "-", "–", "-", "—", "-",
	"π", "pi", "τ", "tau", "φ", "phi",
	"√", "sqrt",
	"²", "^2", "³", "^3",
	"==", "=",
	"[", "(", "]", ")", "{", "(", "}", ")",
)

// "2 x 3" or "2x3" means multiplication when there is no equation.
var timesX = regexp.MustCompile(`(\d)\s*[xX]\s*(\d)`)

func normalize(s string) string {
	s = replacer.Replace(strings.TrimSpace(s))
	if !strings.Contains(s, "=") {
		for timesX.MatchString(s) {
			s = timesX.ReplaceAllString(s, "$1*$2")
		}
	}
	return s
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

func lex(s string) ([]token, error) {
	r := []rune(s)
	var toks []token
	var callStack []bool // per open paren: is it a function call?

	for i := 0; i < len(r); {
		c := r[i]
		switch {
		case unicode.IsSpace(c):
			i++

		case isDigit(c) || (c == '.' && i+1 < len(r) && isDigit(r[i+1])):
			start := i
			var b strings.Builder
			digits := func() {
				for i < len(r) && isDigit(r[i]) {
					b.WriteRune(r[i])
					i++
				}
			}
			digits()
			inCall := len(callStack) > 0 && callStack[len(callStack)-1]
			if i+1 < len(r) && (r[i] == '.' || (r[i] == ',' && !inCall)) && isDigit(r[i+1]) {
				b.WriteByte('.')
				i++
				digits()
			}
			if i < len(r) && (r[i] == 'e' || r[i] == 'E') {
				j := i + 1
				if j < len(r) && (r[j] == '+' || r[j] == '-') {
					j++
				}
				if j < len(r) && isDigit(r[j]) {
					b.WriteByte('e')
					b.WriteString(string(r[i+1 : j]))
					i = j
					digits()
				}
			}
			v, err := strconv.ParseFloat(b.String(), 64)
			if err != nil {
				return nil, fmt.Errorf("número no válido %q", b.String())
			}
			toks = append(toks, token{k: tNum, v: v, s: b.String(), pos: start})

		case unicode.IsLetter(c) || c == '_':
			start := i
			for i < len(r) && (unicode.IsLetter(r[i]) || r[i] == '_') {
				i++
			}
			name := strings.ToLower(string(r[start:i]))
			// log2, log10, atan2...
			j := i
			for j < len(r) && isDigit(r[j]) {
				j++
			}
			if j > i {
				if _, ok := functions[name+string(r[i:j])]; ok {
					name += string(r[i:j])
					i = j
				}
			}
			switch name {
			case "de", "of":
				toks = append(toks, token{k: tOp, s: "*", pos: start})
			case "mod":
				toks = append(toks, token{k: tOp, s: "mod", pos: start})
			default:
				toks = append(toks, token{k: tIdent, s: name, pos: start})
			}

		case c == '(':
			call := false
			if n := len(toks); n > 0 && toks[n-1].k == tIdent {
				_, call = functions[toks[n-1].s]
			}
			callStack = append(callStack, call)
			toks = append(toks, token{k: tLParen, s: "(", pos: i})
			i++

		case c == ')':
			if len(callStack) > 0 {
				callStack = callStack[:len(callStack)-1]
			}
			toks = append(toks, token{k: tRParen, s: ")", pos: i})
			i++

		case c == ',' || c == ';':
			toks = append(toks, token{k: tComma, s: string(c), pos: i})
			i++

		case strings.ContainsRune("+-*/^=!%°", c):
			toks = append(toks, token{k: tOp, s: string(c), pos: i})
			i++

		default:
			return nil, fmt.Errorf("carácter no reconocido %q", string(c))
		}
	}
	return append(toks, token{k: tEOF, pos: len(r)}), nil
}
