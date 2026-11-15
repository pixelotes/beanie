package calc

import (
	"fmt"
	"math"
	"sort"
)

// node is evaluated with the value of the single unknown (ignored if none).
type node interface{ eval(x float64) float64 }

type (
	numNode  float64
	varNode  struct{}
	negNode  struct{ a node }
	pctNode  struct{ a node } // "a%": a/100, or relative in a +/- context
	factNode struct{ a node }
	binNode  struct {
		op   string
		a, b node
	}
	addPctNode struct { // a ± a*p/100
		sign float64
		a, p node
	}
	callNode struct {
		f    function
		args []node
	}
)

func (n numNode) eval(float64) float64    { return float64(n) }
func (varNode) eval(x float64) float64    { return x }
func (n negNode) eval(x float64) float64  { return -n.a.eval(x) }
func (n pctNode) eval(x float64) float64  { return n.a.eval(x) / 100 }
func (n factNode) eval(x float64) float64 { return factorial(n.a.eval(x)) }
func (n addPctNode) eval(x float64) float64 {
	a := n.a.eval(x)
	return a + n.sign*a*n.p.eval(x)/100
}
func (n callNode) eval(x float64) float64 {
	args := make([]float64, len(n.args))
	for i, a := range n.args {
		args[i] = a.eval(x)
	}
	return n.f.f(args)
}
func (n binNode) eval(x float64) float64 {
	a, b := n.a.eval(x), n.b.eval(x)
	switch n.op {
	case "+":
		return a + b
	case "-":
		return a - b
	case "*":
		return a * b
	case "/":
		return a / b
	case "^":
		return math.Pow(a, b)
	case "mod":
		return math.Mod(a, b)
	}
	return math.NaN()
}

type parser struct {
	t    []token
	i    int
	vars map[string]bool
}

func (p *parser) peek() token { return p.t[p.i] }
func (p *parser) next() token { t := p.t[p.i]; p.i++; return t }
func (p *parser) isOp(s ...string) bool {
	t := p.peek()
	if t.k != tOp {
		return false
	}
	for _, o := range s {
		if t.s == o {
			return true
		}
	}
	return false
}

func (p *parser) errAt(msg string) error {
	t := p.peek()
	if t.k == tEOF {
		return fmt.Errorf("%s al final de la expresión", msg)
	}
	return fmt.Errorf("%s cerca de %q (posición %d)", msg, tokenText(t), t.pos+1)
}

func tokenText(t token) string {
	if t.k == tNum && t.s == "" {
		return fmt.Sprint(t.v)
	}
	return t.s
}

// parse returns lhs and, for equations, rhs (nil otherwise).
func parse(input string) (lhs, rhs node, vars []string, err error) {
	toks, err := lex(normalize(input))
	if err != nil {
		return nil, nil, nil, err
	}
	if len(toks) == 1 {
		return nil, nil, nil, fmt.Errorf("expresión vacía")
	}
	p := &parser{t: toks, vars: map[string]bool{}}
	if lhs, err = p.expr(); err != nil {
		return nil, nil, nil, err
	}
	if p.isOp("=") {
		p.next()
		if rhs, err = p.expr(); err != nil {
			return nil, nil, nil, err
		}
		if p.isOp("=") {
			return nil, nil, nil, fmt.Errorf("solo se admite un signo '='")
		}
	}
	if t := p.peek(); t.k != tEOF {
		if t.k == tRParen {
			return nil, nil, nil, p.errAt("paréntesis de cierre sin abrir")
		}
		return nil, nil, nil, p.errAt("símbolo inesperado")
	}
	for v := range p.vars {
		vars = append(vars, v)
	}
	sort.Strings(vars)
	return lhs, rhs, vars, nil
}

func (p *parser) expr() (node, error) {
	left, err := p.mult()
	if err != nil {
		return nil, err
	}
	for p.isOp("+", "-") {
		op := p.next().s
		right, err := p.mult()
		if err != nil {
			return nil, err
		}
		if pc, ok := right.(pctNode); ok {
			sign := 1.0
			if op == "-" {
				sign = -1
			}
			left = addPctNode{sign: sign, a: left, p: pc.a}
			continue
		}
		left = binNode{op: op, a: left, b: right}
	}
	return left, nil
}

func (p *parser) mult() (node, error) {
	left, err := p.unary()
	if err != nil {
		return nil, err
	}
	for {
		var op string
		var right node
		switch t := p.peek(); {
		case p.isOp("*", "/", "mod", "%"):
			op = p.next().s
			if op == "%" {
				op = "mod"
			}
			right, err = p.unary()
		case t.k == tNum || t.k == tIdent || t.k == tLParen: // implicit: 2x, 2(3), (1)(2)
			op = "*"
			right, err = p.power()
		default:
			return left, nil
		}
		if err != nil {
			return nil, err
		}
		left = binNode{op: op, a: left, b: right}
	}
}

func (p *parser) unary() (node, error) {
	if p.isOp("-") {
		p.next()
		a, err := p.unary()
		if err != nil {
			return nil, err
		}
		return negNode{a}, nil
	}
	if p.isOp("+") {
		p.next()
		return p.unary()
	}
	return p.power()
}

func (p *parser) power() (node, error) {
	base, err := p.postfix()
	if err != nil {
		return nil, err
	}
	if p.isOp("^") {
		p.next()
		exp, err := p.unary() // right-associative, allows 2^-1
		if err != nil {
			return nil, err
		}
		return binNode{op: "^", a: base, b: exp}, nil
	}
	return base, nil
}

func (p *parser) postfix() (node, error) {
	n, err := p.primary()
	if err != nil {
		return nil, err
	}
	for {
		switch {
		case p.isOp("!"):
			p.next()
			n = factNode{n}
		case p.isOp("°"):
			p.next()
			n = binNode{op: "*", a: n, b: numNode(math.Pi / 180)}
		case p.isOp("%"):
			// "7 % 3" is modulo; "50%", "10% de 80", "100 + 10%" are percentages.
			if nx := p.t[p.i+1]; nx.k == tNum || nx.k == tIdent || nx.k == tLParen {
				return n, nil
			}
			p.next()
			n = pctNode{n}
		default:
			return n, nil
		}
	}
}

func (p *parser) primary() (node, error) {
	t := p.peek()
	switch t.k {
	case tNum:
		p.next()
		return numNode(t.v), nil

	case tLParen:
		p.next()
		n, err := p.expr()
		if err != nil {
			return nil, err
		}
		if p.peek().k != tRParen {
			return nil, p.errAt("falta cerrar un paréntesis")
		}
		p.next()
		return n, nil

	case tIdent:
		p.next()
		if c, ok := constants[t.s]; ok {
			return numNode(c), nil
		}
		if f, ok := functions[t.s]; ok {
			return p.call(t, f)
		}
		if len([]rune(t.s)) == 1 {
			p.vars[t.s] = true
			return varNode{}, nil
		}
		return nil, fmt.Errorf("función o constante desconocida %q (las incógnitas deben ser una sola letra)", t.s)

	case tEOF:
		return nil, p.errAt("falta un operando")
	}
	return nil, p.errAt("símbolo inesperado")
}

func (p *parser) call(name token, f function) (node, error) {
	var args []node
	if p.peek().k != tLParen { // "sin x", "sqrt 2"
		a, err := p.power()
		if err != nil {
			return nil, err
		}
		args = []node{a}
	} else {
		p.next()
		for {
			a, err := p.expr()
			if err != nil {
				return nil, err
			}
			args = append(args, a)
			if p.peek().k == tComma {
				p.next()
				continue
			}
			if p.peek().k != tRParen {
				return nil, p.errAt(fmt.Sprintf("falta cerrar el paréntesis de %s(", name.s))
			}
			p.next()
			break
		}
	}
	if len(args) < f.min || (f.max >= 0 && len(args) > f.max) {
		want := fmt.Sprint(f.min)
		switch {
		case f.max < 0:
			want = fmt.Sprintf("al menos %d", f.min)
		case f.max != f.min:
			want = fmt.Sprintf("entre %d y %d", f.min, f.max)
		}
		return nil, fmt.Errorf("%s() espera %s argumento(s) y recibió %d", name.s, want, len(args))
	}
	return callNode{f: f, args: args}, nil
}
