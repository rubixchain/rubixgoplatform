package coin

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

const MaxSupportedDecimalPlaces int = 3

var (
	ErrInvalidFraction = errors.New("fraction must be numeric with at most 3 digits")
)

func normalizeFraction(f string) (string, error) {
	if f == "" {
		return "000", nil
	}

	if len(f) > MaxSupportedDecimalPlaces {
		return "", ErrInvalidFraction
	}

	for _, r := range f {
		if !unicode.IsDigit(r) {
			return "", ErrInvalidFraction
		}
	}

	// Right-pad with zeros to 3 digits
	if len(f) < MaxSupportedDecimalPlaces {
		f = f + strings.Repeat("0", MaxSupportedDecimalPlaces-len(f))
	}

	return f, nil
}

func coinFromMinorUnits(v int64) (*Coin, error) {
	if v < 0 {
		return nil, errors.New("coin value cannot be negative")
	}

	multiplier := int64(1)
	for i := 0; i < MaxSupportedDecimalPlaces; i++ {
		multiplier *= 10
	}

	whole := v / multiplier
	frac := v % multiplier

	return &Coin{
		Whole:    whole,
		Fraction: fmt.Sprintf("%0*d", MaxSupportedDecimalPlaces, frac),
	}, nil
}

type Coin struct {
	Whole    int64
	Fraction string
}

func NewCoin(whole int64, fraction string) (*Coin, error) {
	if whole < 0 {
		return nil, fmt.Errorf("whole component cannot be negative, provided value: %v", whole)
	}

	frac, err := normalizeFraction(fraction)
	if err != nil {
		return nil, err
	}

	return &Coin{
		Whole:    whole,
		Fraction: frac,
	}, nil
}

func NewCoinFromFloat64(v float64) (*Coin, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil, errors.New("invalid float value")
	}

	if v < 0 {
		return nil, errors.New("coin value cannot be negative")
	}

	multiplier := math.Pow10(MaxSupportedDecimalPlaces)

	// Scale
	scaled := v * multiplier

	// Reject if not exactly representable
	if math.Trunc(scaled) != scaled {
		return nil, errors.New("float has more precision than supported")
	}

	// Guard int64 overflow
	if scaled > float64(math.MaxInt64) {
		return nil, errors.New("coin value overflow")
	}

	return coinFromMinorUnits(int64(scaled))
}

func NewCoinFromString(s string) (*Coin, error) {
	if s == "" {
		return nil, errors.New("empty string is not a valid coin value")
	}

	var wholePart, fracPart string
	dotSeen := false

	for i, r := range s {
		switch {
		case r == '.':
			if dotSeen {
				return nil, errors.New("invalid coin format: multiple decimal points")
			}
			if i == 0 || i == len(s)-1 {
				return nil, errors.New("invalid coin format: decimal point position")
			}
			dotSeen = true
		case r >= '0' && r <= '9':
			// ok
		default:
			return nil, errors.New("invalid coin format: non-numeric character")
		}
	}

	if dotSeen {
		parts := strings.Split(s, ".")
		if len(parts) != 2 {
			return nil, errors.New("invalid coin format")
		}
		wholePart = parts[0]
		fracPart = parts[1]
	} else {
		wholePart = s
		fracPart = ""
	}

	// Reject leading zeros in whole part (except "0")
	if len(wholePart) > 1 && wholePart[0] == '0' {
		return nil, errors.New("invalid coin format: leading zeros not allowed")
	}

	whole, err := strconv.ParseInt(wholePart, 10, 64)
	if err != nil {
		return nil, errors.New("invalid whole component")
	}

	if whole < 0 {
		return nil, errors.New("coin value cannot be negative")
	}

	return NewCoin(whole, fracPart)
}

func (c Coin) toMinorUnits() (int64, error) {
	var frac int64
	for _, r := range c.Fraction {
		frac = frac*10 + int64(r-'0')
	}

	multiplier := int64(1)
	for i := 0; i < MaxSupportedDecimalPlaces; i++ {
		multiplier *= 10
	}

	// overflow guard
	if c.Whole > (int64(^uint64(0)>>1) / multiplier) {
		return 0, errors.New("coin value overflow")
	}

	return c.Whole*multiplier + frac, nil
}

func (c Coin) String() string {
	return fmt.Sprintf("%d.%s", c.Whole, c.Fraction)
}

func (c Coin) Cmp(other Coin) (int, error) {
	a, err := c.toMinorUnits()
	if err != nil {
		return 0, err
	}

	b, err := other.toMinorUnits()
	if err != nil {
		return 0, err
	}

	switch {
	case a < b:
		return -1, nil
	case a > b:
		return 1, nil
	default:
		return 0, nil
	}
}

func (c Coin) Equal(o Coin) bool {
	r, _ := c.Cmp(o)
	return r == 0
}

func (c Coin) LessThan(o Coin) bool {
	r, _ := c.Cmp(o)
	return r < 0
}

func (c Coin) Add(other Coin) (*Coin, error) {
	a, err := c.toMinorUnits()
	if err != nil {
		return nil, err
	}

	b, err := other.toMinorUnits()
	if err != nil {
		return nil, err
	}

	return coinFromMinorUnits(a + b)
}

func (c Coin) Sub(other Coin) (*Coin, error) {
	a, err := c.toMinorUnits()
	if err != nil {
		return nil, err
	}

	b, err := other.toMinorUnits()
	if err != nil {
		return nil, err
	}

	if a < b {
		return nil, errors.New("coin subtraction would result in negative value")
	}

	return coinFromMinorUnits(a - b)
}

func (c Coin) IsZero() bool {
	for _, r := range c.Fraction {
		if r != '0' {
			return false
		}
	}
	return c.Whole == 0
}
