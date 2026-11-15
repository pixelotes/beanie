package calc

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

const maxRoots = 8

// Evaluate computes an expression ("2+2", "20% de 150", "sin(30°)") or solves
// a one-unknown equation ("2x + 3 = 11", "x^2 = 2", "2^x = 10").
func Evaluate(input string) (string, error) {
	lhs, rhs, vars, err := parse(input)
	if err != nil {
		return "", err
	}
	if len(vars) > 1 {
		return "", fmt.Errorf("solo se admite una incógnita y hay %d: %s", len(vars), strings.Join(vars, ", "))
	}

	if rhs == nil {
		if len(vars) == 1 {
			return "", fmt.Errorf("la expresión contiene la incógnita %s; escribe una ecuación con '=' para resolverla", vars[0])
		}
		v := lhs.eval(0)
		if err := checkValue(v); err != nil {
			return "", err
		}
		return Format(v), nil
	}

	if len(vars) == 0 {
		a, b := lhs.eval(0), rhs.eval(0)
		if err := checkValue(a); err != nil {
			return "", err
		}
		if err := checkValue(b); err != nil {
			return "", err
		}
		if almostEqual(a, b) {
			return fmt.Sprintf("Verdadero (%s = %s)", Format(a), Format(b)), nil
		}
		return fmt.Sprintf("Falso (%s ≠ %s)", Format(a), Format(b)), nil
	}

	name := vars[0]
	f := func(x float64) float64 { return lhs.eval(x) - rhs.eval(x) }
	if res, ok := solvePolynomial(f, name); ok {
		return res, nil
	}
	return solveNumeric(f, name), nil
}

func checkValue(v float64) error {
	switch {
	case math.IsNaN(v):
		return fmt.Errorf("resultado indefinido (por ejemplo raíz o logaritmo de un negativo, 0/0 o factorial no entero)")
	case math.IsInf(v, 0):
		return fmt.Errorf("resultado infinito (división por cero o número demasiado grande)")
	}
	return nil
}

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) <= 1e-9*math.Max(1, math.Max(math.Abs(a), math.Abs(b)))
}

// solvePolynomial detects degree ≤ 2 by fitting a quadratic on three points
// and checking it on several others, then solves exactly.
func solvePolynomial(f func(float64) float64, name string) (string, bool) {
	f0, f1, fm := f(0), f(1), f(-1)
	for _, v := range []float64{f0, f1, fm} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return "", false
		}
	}
	c := f0
	a := (f1+fm)/2 - c
	b := (f1 - fm) / 2
	for _, x := range []float64{2, -3, 0.5, 7, -11, 0.25} {
		y := f(x)
		if math.IsNaN(y) || !almostEqual(y, a*x*x+b*x+c) {
			return "", false
		}
	}
	scale := math.Max(1, math.Max(math.Abs(b), math.Abs(c)))
	if math.Abs(a) < 1e-12*scale {
		a = 0
	}
	if a == 0 && math.Abs(b) < 1e-12*math.Max(1, math.Abs(c)) {
		b = 0
	}

	switch {
	case a == 0 && b == 0:
		if math.Abs(c) < 1e-12 {
			return fmt.Sprintf("Cualquier valor de %s es solución (la ecuación es una identidad)", name), true
		}
		return "No tiene solución (la ecuación es una contradicción)", true

	case a == 0:
		return fmt.Sprintf("%s = %s", name, Format(snap(-c/b))), true
	}

	d := b*b - 4*a*c
	if math.Abs(d) < 1e-12*math.Max(1, b*b) {
		return fmt.Sprintf("%s = %s (raíz doble)", name, Format(snap(-b/(2*a)))), true
	}
	if d < 0 {
		re, im := -b/(2*a), math.Sqrt(-d)/(2*math.Abs(a))
		return fmt.Sprintf("No tiene soluciones reales (complejas: %s = %s ± %si)", name, Format(snap(re)), Format(snap(im))), true
	}
	// Numerically stable quadratic formula.
	q := -0.5 * (b + math.Copysign(math.Sqrt(d), b))
	r1, r2 := q/a, c/q
	if r1 > r2 {
		r1, r2 = r2, r1
	}
	return fmt.Sprintf("%s = %s, %s = %s", name, Format(snap(r1)), name, Format(snap(r2))), true
}

var grid []float64

func init() {
	for i := -10000; i <= 10000; i++ { // [-10, 10] step 0.001
		grid = append(grid, float64(i)/1000)
	}
	for i := 201; i <= 20000; i++ { // (10, 1000] step 0.05
		grid = append(grid, float64(i)*0.05, -float64(i)*0.05)
	}
	for i := 1; i <= 2000; i++ { // (1000, 1e6] geometric
		v := 1000 * math.Pow(1000, float64(i)/2000)
		grid = append(grid, v, -v)
	}
	sort.Float64s(grid)
}

func solveNumeric(f func(float64) float64, name string) string {
	var roots []float64
	prevX, prevY := math.NaN(), math.NaN()
	for _, x := range grid {
		y := f(x)
		if math.IsNaN(y) || math.IsInf(y, 0) {
			prevX, prevY = math.NaN(), math.NaN()
			continue
		}
		if y == 0 {
			roots = append(roots, x)
		} else if !math.IsNaN(prevY) && prevY != 0 && (prevY < 0) != (y < 0) {
			if r, ok := bisect(f, prevX, x, prevY); ok {
				roots = append(roots, r)
			}
		}
		prevX, prevY = x, y
	}

	roots = dedupe(roots)
	if len(roots) == 0 {
		return fmt.Sprintf("No se encontraron soluciones reales para %s en [-1e6, 1e6]", name)
	}
	sort.Slice(roots, func(i, j int) bool { return math.Abs(roots[i]) < math.Abs(roots[j]) })
	more := len(roots) > maxRoots
	if more {
		roots = roots[:maxRoots]
	}
	sort.Float64s(roots)

	parts := make([]string, len(roots))
	for i, r := range roots {
		parts[i] = fmt.Sprintf("%s ≈ %s", name, Format(snap(r)))
	}
	out := strings.Join(parts, ", ")
	if more {
		out += fmt.Sprintf(" (y más; se muestran las %d más cercanas a 0, puede haber infinitas)", maxRoots)
	}
	return out + " [solución numérica]"
}

func bisect(f func(float64) float64, a, b, fa float64) (float64, bool) {
	for i := 0; i < 200 && b-a > 0; i++ {
		m := a + (b-a)/2
		if m == a || m == b {
			break
		}
		fm := f(m)
		if math.IsNaN(fm) {
			return 0, false
		}
		if fm == 0 {
			return m, true
		}
		if (fm < 0) == (fa < 0) {
			a, fa = m, fm
		} else {
			b = m
		}
	}
	r := a + (b-a)/2
	// Reject sign changes caused by poles (tan(x) = 0 at π/2, 1/x...).
	return r, math.Abs(f(r)) < 1e-6
}

func dedupe(r []float64) []float64 {
	sort.Float64s(r)
	var out []float64
	for _, v := range r {
		if len(out) > 0 && math.Abs(v-out[len(out)-1]) <= 1e-7*math.Max(1, math.Abs(v)) {
			continue
		}
		out = append(out, v)
	}
	return out
}

// snap rounds values that are integers up to floating-point noise.
func snap(v float64) float64 {
	if r := math.Round(v); math.Abs(v-r) < 1e-9*math.Max(1, math.Abs(v)) {
		return r
	}
	return v
}

// Format prints a number without floating-point noise (0.1+0.2 → 0.3).
func Format(v float64) string {
	if v == 0 {
		return "0"
	}
	if v == math.Trunc(v) && math.Abs(v) < 1e15 {
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	s := strconv.FormatFloat(v, 'g', 12, 64)
	if strings.ContainsAny(s, "e") {
		return s
	}
	return strconv.FormatFloat(mustParse(s), 'f', -1, 64)
}

func mustParse(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}
