package calc

import (
	"strings"
	"testing"
)

func TestEvaluate(t *testing.T) {
	cases := []struct{ in, want string }{
		// Spacing and operator variants
		{"2+2", "4"},
		{"2 + 2", "4"},
		{"  2  +2 ", "4"},
		{"2 × 3", "6"},
		{"2x3", "6"},
		{"2 x 3", "6"},
		{"6 ÷ 4", "1.5"},
		{"6 : 4", ""}, // ':' not supported → error
		{"2**10", "1024"},
		{"2^10", "1024"},
		{"2^-1", "0.5"},
		{"2^3^2", "512"},
		{"-2^2", "-4"},
		{"5 − 3", "2"},
		{"7 % 3", "1"},
		{"7 mod 3", "1"},
		// Decimals
		{"0.1 + 0.2", "0.3"},
		{"2,5 * 2", "5"},
		{"max(2,5)", "5"},
		{"max(2.5; 3)", "3"},
		{".5 + .5", "1"},
		{"1e3 + 1", "1001"},
		{"2.5e-3 * 1000", "2.5"},
		// Implicit multiplication
		{"2(3+1)", "8"},
		{"(1+2)(3+4)", "21"},
		{"2pi", "6.28318530718"},
		{"2π", "6.28318530718"},
		// Functions and constants
		{"sqrt(16)", "4"},
		{"√16", "4"},
		{"sqrt 16 + 1", "5"},
		{"3²", "9"},
		{"sin(30°)", "0.5"},
		{"sen(90°)", "1"},
		{"cos(pi)", "-1"},
		{"ln(e)", "1"},
		{"log(1000)", "3"},
		{"log(8, 2)", "3"},
		{"log2(8)", "3"},
		{"5!", "120"},
		{"fact(5)", "120"},
		{"round(3.14159, 2)", "3.14"},
		{"abs(-3)", "3"},
		{"mcd(12, 18)", "6"},
		{"lcm(4, 6)", "12"},
		{"[2 + {3}] * 2", "10"},
		// Percentages
		{"50%", "0.5"},
		{"20% de 150", "30"},
		{"20% of 150", "30"},
		{"150 + 10%", "165"},
		{"200 - 25%", "150"},
		{"10% * 80", "8"},
		// Equations
		{"2x + 3 = 11", "x = 4"},
		{"3(y - 1) = 2y + 5", "y = 8"},
		{"x/4 = 2,5", "x = 10"},
		{"x^2 - 5x + 6 = 0", "x = 2, x = 3"},
		{"x² = 2", "x = -1.41421356237, x = 1.41421356237"},
		{"x^2 - 6x + 9 = 0", "x = 3 (raíz doble)"},
		{"x^2 + 1 = 0", "No tiene soluciones reales (complejas: x = 0 ± 1i)"},
		{"x + 1 = x + 1", "Cualquier valor de x es solución (la ecuación es una identidad)"},
		{"x + 1 = x", "No tiene solución (la ecuación es una contradicción)"},
		{"2^x = 1024", "x ≈ 10 [solución numérica]"},
		{"x^3 = 27", "x ≈ 3 [solución numérica]"},
		{"1/x = 4", "x ≈ 0.25 [solución numérica]"},
		{"sqrt(x) = 3", "x ≈ 9 [solución numérica]"},
		{"2 + 2 = 4", "Verdadero (4 = 4)"},
		{"2 + 2 == 5", "Falso (4 ≠ 5)"},
	}
	for _, c := range cases {
		got, err := Evaluate(c.in)
		if c.want == "" {
			if err == nil {
				t.Errorf("%q: expected error, got %q", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: unexpected error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("%q = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPeriodicAndPoles(t *testing.T) {
	got, err := Evaluate("sin(x) = 0.5")
	if err != nil || !strings.Contains(got, "x ≈ 0.523598775598") || !strings.Contains(got, "y más") {
		t.Errorf("sin(x)=0.5 → %q, %v", got, err)
	}
	got, err = Evaluate("tan(x) = 0")
	if err != nil || strings.Contains(got, "1.5707") {
		t.Errorf("tan(x)=0 should not report the pole: %q, %v", got, err)
	}
}

func TestErrors(t *testing.T) {
	cases := map[string]string{
		"":          "vacía",
		"2 +":       "falta un operando",
		"(2 + 3":    "falta cerrar",
		"2 + 3)":    "sin abrir",
		"foo(2)":    "desconocida",
		"x + y = 2": "una incógnita",
		"2x + 1":    "escribe una ecuación",
		"1/0":       "infinito",
		"sqrt(-1)":  "indefinido",
		"max()":     "inesperado",
		"pow(2)":    "espera 2",
		"2 $ 3":     "no reconocido",
		"x = 1 = 2": "un signo",
	}
	for in, want := range cases {
		_, err := Evaluate(in)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: error %v, want containing %q", in, err, want)
		}
	}
}
