package calc

import "math"

type function struct {
	min, max int // max < 0: variadic
	f        func(a []float64) float64
}

func f1(f func(float64) float64) function {
	return function{1, 1, func(a []float64) float64 { return f(a[0]) }}
}

var constants = map[string]float64{
	"pi":  math.Pi,
	"e":   math.E,
	"tau": 2 * math.Pi,
	"phi": math.Phi,
}

var functions map[string]function

func init() {
	functions = map[string]function{
		"sqrt":   f1(math.Sqrt),
		"raiz":   f1(math.Sqrt),
		"cbrt":   f1(math.Cbrt),
		"abs":    f1(math.Abs),
		"floor":  f1(math.Floor),
		"ceil":   f1(math.Ceil),
		"trunc":  f1(math.Trunc),
		"sin":    f1(math.Sin),
		"sen":    f1(math.Sin),
		"cos":    f1(math.Cos),
		"tan":    f1(math.Tan),
		"tg":     f1(math.Tan),
		"asin":   f1(math.Asin),
		"arcsin": f1(math.Asin),
		"asen":   f1(math.Asin),
		"acos":   f1(math.Acos),
		"arccos": f1(math.Acos),
		"atan":   f1(math.Atan),
		"arctan": f1(math.Atan),
		"sinh":   f1(math.Sinh),
		"cosh":   f1(math.Cosh),
		"tanh":   f1(math.Tanh),
		"ln":     f1(math.Log),
		"log2":   f1(math.Log2),
		"log10":  f1(math.Log10),
		"exp":    f1(math.Exp),
		"fact":   f1(factorial),
		"rad":    f1(func(x float64) float64 { return x * math.Pi / 180 }),
		"deg":    f1(func(x float64) float64 { return x * 180 / math.Pi }),
		"sign": f1(func(x float64) float64 {
			switch {
			case x > 0:
				return 1
			case x < 0:
				return -1
			}
			return 0
		}),
		"log": {1, 2, func(a []float64) float64 {
			if len(a) == 2 {
				return math.Log(a[0]) / math.Log(a[1])
			}
			return math.Log10(a[0])
		}},
		"round": {1, 2, func(a []float64) float64 {
			if len(a) == 1 {
				return math.Round(a[0])
			}
			p := math.Pow(10, math.Trunc(a[1]))
			return math.Round(a[0]*p) / p
		}},
		"pow":   {2, 2, func(a []float64) float64 { return math.Pow(a[0], a[1]) }},
		"atan2": {2, 2, func(a []float64) float64 { return math.Atan2(a[0], a[1]) }},
		"hypot": {2, 2, func(a []float64) float64 { return math.Hypot(a[0], a[1]) }},
		"min": {1, -1, func(a []float64) float64 {
			m := a[0]
			for _, v := range a[1:] {
				m = math.Min(m, v)
			}
			return m
		}},
		"max": {1, -1, func(a []float64) float64 {
			m := a[0]
			for _, v := range a[1:] {
				m = math.Max(m, v)
			}
			return m
		}},
		"gcd": {2, -1, intFold(gcd)},
		"mcd": {2, -1, intFold(gcd)},
		"lcm": {2, -1, intFold(func(a, b float64) float64 {
			if a == 0 || b == 0 {
				return 0
			}
			return math.Abs(a*b) / gcd(a, b)
		})},
		"mcm": {2, -1, intFold(func(a, b float64) float64 {
			if a == 0 || b == 0 {
				return 0
			}
			return math.Abs(a*b) / gcd(a, b)
		})},
	}
}

func factorial(x float64) float64 {
	if x < 0 || x != math.Trunc(x) {
		return math.NaN()
	}
	if x > 170 {
		return math.Inf(1)
	}
	return math.Round(math.Gamma(x + 1))
}

func gcd(a, b float64) float64 {
	a, b = math.Abs(a), math.Abs(b)
	for b != 0 {
		a, b = b, math.Mod(a, b)
	}
	return a
}

func intFold(op func(a, b float64) float64) func([]float64) float64 {
	return func(a []float64) float64 {
		for _, v := range a {
			if v != math.Trunc(v) {
				return math.NaN()
			}
		}
		acc := a[0]
		for _, v := range a[1:] {
			acc = op(acc, v)
		}
		return acc
	}
}
