package solanatools

import (
	"fmt"
	"strconv"
	"strings"
)

// LamportsPerSOL is the number of integer lamports in one SOL.
const LamportsPerSOL uint64 = 1_000_000_000

// ParseAmount converts a nonnegative decimal string to raw units without rounding.
// Exponents, signs and excess fractional digits are rejected.
func ParseAmount(value string, decimals uint8) (uint64, error) {
	if value == "" {
		return 0, fmt.Errorf("amount is empty")
	}
	parts := strings.Split(value, ".")
	if len(parts) > 2 || parts[0] == "" {
		return 0, fmt.Errorf("invalid amount %q", value)
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
		if fraction == "" {
			return 0, fmt.Errorf("missing fractional digits")
		}
	}
	for _, c := range parts[0] + fraction {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid amount %q", value)
		}
	}
	if len(fraction) > int(decimals) {
		return 0, fmt.Errorf("amount exceeds %d decimal places", decimals)
	}
	digits := strings.TrimLeft(parts[0]+fraction+strings.Repeat("0", int(decimals)-len(fraction)), "0")
	if digits == "" {
		return 0, nil
	}
	raw, err := strconv.ParseUint(digits, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("amount overflow: %w", err)
	}
	return raw, nil
}

// ParseSOL converts an exact decimal SOL string to lamports without rounding.
func ParseSOL(value string) (uint64, error) { return ParseAmount(value, 9) }

// FormatAmount returns an exact decimal string, removing trailing fractional zeros.
func FormatAmount(raw uint64, decimals uint8) string {
	digits := strconv.FormatUint(raw, 10)
	if decimals == 0 {
		return digits
	}
	if len(digits) <= int(decimals) {
		digits = strings.Repeat("0", int(decimals)+1-len(digits)) + digits
	}
	split := len(digits) - int(decimals)
	result := strings.TrimRight(digits[:split]+"."+digits[split:], "0")
	return strings.TrimSuffix(result, ".")
}

// FormatSOL formats lamports as an exact SOL string without trailing zeros.
func FormatSOL(lamports uint64) string { return FormatAmount(lamports, 9) }

// TokenAmount contains raw token units, decimals and an exact display string.
type TokenAmount struct {
	RawAmount uint64
	Decimals  uint8
	UIAmount  string
}

// NewTokenAmount creates a token amount and calculates its exact UIAmount.
func NewTokenAmount(raw uint64, decimals uint8) TokenAmount {
	return TokenAmount{RawAmount: raw, Decimals: decimals, UIAmount: FormatAmount(raw, decimals)}
}
