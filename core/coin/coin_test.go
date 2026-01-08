package coin

import (
	"math"
	"testing"
)

func TestNewCoin_ValidCases(t *testing.T) {
	tests := []struct {
		name     string
		whole    int64
		fraction string
		expWhole int64
		expFrac  string
	}{
		{
			name:     "zero value with empty fraction",
			whole:    0,
			fraction: "",
			expWhole: 0,
			expFrac:  "000",
		},
		{
			name:     "single digit fraction",
			whole:    1,
			fraction: "2",
			expWhole: 1,
			expFrac:  "200",
		},
		{
			name:     "two digit fraction",
			whole:    5,
			fraction: "45",
			expWhole: 5,
			expFrac:  "450",
		},
		{
			name:     "exact precision fraction",
			whole:    10,
			fraction: "123",
			expWhole: 10,
			expFrac:  "123",
		},
		{
			name:     "leading zero fraction preserved",
			whole:    3,
			fraction: "007",
			expWhole: 3,
			expFrac:  "007",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt := tt
			c, err := NewCoin(tt.whole, tt.fraction)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if c.Whole != tt.expWhole {
				t.Fatalf("whole mismatch: expected %d, got %d", tt.expWhole, c.Whole)
			}

			if c.Fraction != tt.expFrac {
				t.Fatalf("fraction mismatch: expected %q, got %q", tt.expFrac, c.Fraction)
			}
		})
	}
}

func TestNewCoin_InvalidWhole(t *testing.T) {
	tests := []int64{-1, -10, -999}

	for _, whole := range tests {
		_, err := NewCoin(whole, "000")
		if err == nil {
			t.Fatalf("expected error for whole=%d, got nil", whole)
		}
	}
}

func TestNewCoin_ZeroWhole_NonZeroFraction(t *testing.T) {
	c, err := NewCoin(0, "5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c.Whole != 0 || c.Fraction != "500" {
		t.Fatalf("expected 0.500, got %d.%s", c.Whole, c.Fraction)
	}
}

func TestNewCoin_InvalidFraction(t *testing.T) {
	tests := []struct {
		name     string
		fraction string
	}{
		{
			name:     "too many decimal places",
			fraction: "1234",
		},
		{
			name:     "non-numeric characters",
			fraction: "1a2",
		},
		{
			name:     "decimal point not allowed",
			fraction: "1.2",
		},
		{
			name:     "negative sign not allowed",
			fraction: "-12",
		},
		{
			name:     "whitespace not allowed",
			fraction: " 12",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewCoin(1, tt.fraction)
			if err == nil {
				t.Fatalf("expected error for invalid fraction %q, got nil", tt.fraction)
			}
		})
	}
}

func TestNormalizeFraction_Idempotent(t *testing.T) {
	input := "120"

	first, err := normalizeFraction(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	second, err := normalizeFraction(first)
	if err != nil {
		t.Fatalf("unexpected error on second normalization: %v", err)
	}

	if first != second {
		t.Fatalf("normalizeFraction not idempotent: %q vs %q", first, second)
	}
}

func TestCoin_ToMinorUnits(t *testing.T) {
	tests := []struct {
		name   string
		coin   Coin
		expect int64
	}{
		{
			name:   "zero coin",
			coin:   Coin{Whole: 0, Fraction: "000"},
			expect: 0,
		},
		{
			name:   "simple value",
			coin:   Coin{Whole: 1, Fraction: "000"},
			expect: 1000,
		},
		{
			name:   "fraction only",
			coin:   Coin{Whole: 0, Fraction: "500"},
			expect: 500,
		},
		{
			name:   "whole and fraction",
			coin:   Coin{Whole: 12, Fraction: "345"},
			expect: 12345,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			v, err := tt.coin.toMinorUnits()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if v != tt.expect {
				t.Fatalf("expected %d, got %d", tt.expect, v)
			}
		})
	}
}

func TestCoin_FromMinorUnits(t *testing.T) {
	tests := []struct {
		name     string
		input    int64
		expWhole int64
		expFrac  string
	}{
		{
			name:     "zero",
			input:    0,
			expWhole: 0,
			expFrac:  "000",
		},
		{
			name:     "fraction only",
			input:    250,
			expWhole: 0,
			expFrac:  "250",
		},
		{
			name:     "whole only",
			input:    3000,
			expWhole: 3,
			expFrac:  "000",
		},
		{
			name:     "whole and fraction",
			input:    12345,
			expWhole: 12,
			expFrac:  "345",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			c, err := coinFromMinorUnits(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if c.Whole != tt.expWhole || c.Fraction != tt.expFrac {
				t.Fatalf(
					"expected %d.%s, got %d.%s",
					tt.expWhole, tt.expFrac,
					c.Whole, c.Fraction,
				)
			}
		})
	}
}

func TestCoin_FromMinorUnits_Negative(t *testing.T) {
	_, err := coinFromMinorUnits(-1)
	if err == nil {
		t.Fatal("expected error for negative minor units, got nil")
	}
}

func TestCoin_String(t *testing.T) {
	c := Coin{Whole: 7, Fraction: "045"}
	if c.String() != "7.045" {
		t.Fatalf("expected 7.045, got %s", c.String())
	}
}

func TestCoin_Comparison(t *testing.T) {
	a := Coin{Whole: 1, Fraction: "000"}
	b := Coin{Whole: 1, Fraction: "500"}
	c := Coin{Whole: 2, Fraction: "000"}

	if r, _ := a.Cmp(b); r >= 0 {
		t.Fatal("expected a < b")
	}

	if r, _ := b.Cmp(a); r <= 0 {
		t.Fatal("expected b > a")
	}

	if r, _ := b.Cmp(b); r != 0 {
		t.Fatal("expected b == b")
	}

	if !a.LessThan(b) {
		t.Fatal("expected a.LessThan(b) == true")
	}

	if !b.Equal(Coin{Whole: 1, Fraction: "500"}) {
		t.Fatal("expected coins to be equal")
	}

	if b.LessThan(a) {
		t.Fatal("expected b not < a")
	}

	if !b.LessThan(c) {
		t.Fatal("expected b < c")
	}
}

func TestCoin_Add(t *testing.T) {
	a := Coin{Whole: 1, Fraction: "900"}
	b := Coin{Whole: 0, Fraction: "200"}

	sum, err := a.Add(b)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sum.Whole != 2 || sum.Fraction != "100" {
		t.Fatalf("expected 2.100, got %d.%s", sum.Whole, sum.Fraction)
	}
}

func TestCoin_Sub(t *testing.T) {
	a := Coin{Whole: 2, Fraction: "000"}
	b := Coin{Whole: 0, Fraction: "001"}

	diff, err := a.Sub(b)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if diff.Whole != 1 || diff.Fraction != "999" {
		t.Fatalf("expected 1.999, got %d.%s", diff.Whole, diff.Fraction)
	}
}

func TestCoin_Sub_NegativeResult(t *testing.T) {
	a := Coin{Whole: 1, Fraction: "000"}
	b := Coin{Whole: 1, Fraction: "001"}

	_, err := a.Sub(b)
	if err == nil {
		t.Fatal("expected error for negative subtraction result, got nil")
	}
}

func TestCoin_IsZero(t *testing.T) {
	tests := []struct {
		coin   Coin
		expect bool
	}{
		{Coin{Whole: 0, Fraction: "000"}, true},
		{Coin{Whole: 0, Fraction: "001"}, false},
		{Coin{Whole: 1, Fraction: "000"}, false},
	}

	for _, tt := range tests {
		tt := tt
		if tt.coin.IsZero() != tt.expect {
			t.Fatalf("IsZero failed for %d.%s", tt.coin.Whole, tt.coin.Fraction)
		}
	}
}

func TestNewCoinFromFloat64_Exact(t *testing.T) {
	tests := []struct {
		name     string
		input    float64
		expWhole int64
		expFrac  string
	}{
		{
			name:     "integer",
			input:    5.0,
			expWhole: 5,
			expFrac:  "000",
		},
		{
			name:     "one decimal place",
			input:    1.2,
			expWhole: 1,
			expFrac:  "200",
		},
		{
			name:     "two decimal places",
			input:    3.45,
			expWhole: 3,
			expFrac:  "450",
		},
		{
			name:     "max precision",
			input:    7.123,
			expWhole: 7,
			expFrac:  "123",
		},
		{
			name:     "fraction only",
			input:    0.5,
			expWhole: 0,
			expFrac:  "500",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewCoinFromFloat64(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if c.Whole != tt.expWhole || c.Fraction != tt.expFrac {
				t.Fatalf(
					"expected %d.%s, got %d.%s",
					tt.expWhole, tt.expFrac,
					c.Whole, c.Fraction,
				)
			}
		})
	}
}

func TestNewCoinFromFloat64_Rejected(t *testing.T) {
	tests := []float64{
		1.2341,       // too many decimals
		1.0000000001, // float noise
		-1.0,         // negative
		math.NaN(),   // NaN
		math.Inf(1),  // +Inf
		math.Inf(-1), // -Inf
	}

	for _, v := range tests {
		_, err := NewCoinFromFloat64(v)
		if err == nil {
			t.Fatalf("expected error for input %v, got nil", v)
		}
	}
}

func TestNewCoinFromString_Valid(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expWhole int64
		expFrac  string
	}{
		{"integer", "5", 5, "000"},
		{"zero", "0", 0, "000"},
		{"simple decimal", "1.2", 1, "200"},
		{"two decimals", "3.45", 3, "450"},
		{"max precision", "7.123", 7, "123"},
		{"zero whole with fraction", "0.5", 0, "500"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewCoinFromString(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if c.Whole != tt.expWhole || c.Fraction != tt.expFrac {
				t.Fatalf(
					"expected %d.%s, got %d.%s",
					tt.expWhole, tt.expFrac,
					c.Whole, c.Fraction,
				)
			}
		})
	}
}

func TestNewCoinFromString_Invalid(t *testing.T) {
	tests := []string{
		"",
		".5",
		"1.",
		"1.2345",
		"1.2.3",
		"1,23",
		"1e3",
		" 1.2",
		"-1.2",
		"-5",
		"01",
		"001",
		"01.2",
		"0001.005",
		"00.5",
	}

	for _, s := range tests {
		_, err := NewCoinFromString(s)
		if err == nil {
			t.Fatalf("expected error for input %q, got nil", s)
		}
	}
}

