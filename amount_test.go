package solanatools

import "testing"

func TestAmounts(t *testing.T) {
	for _, tc := range []struct {
		text     string
		decimals uint8
		raw      uint64
	}{
		{"0", 9, 0}, {"0.125", 9, 125000000}, {"12.5", 6, 12500000}, {"18446744073709551615", 0, ^uint64(0)}, {"000.001", 3, 1},
	} {
		raw, err := ParseAmount(tc.text, tc.decimals)
		if err != nil || raw != tc.raw {
			t.Fatalf("%s: %d, %v", tc.text, raw, err)
		}
		parsed, err := ParseAmount(FormatAmount(raw, tc.decimals), tc.decimals)
		if err != nil || parsed != raw {
			t.Fatalf("roundtrip: %d %v", parsed, err)
		}
	}
	for _, value := range []string{"-1", "+1", "NaN", "1e9", "", ".1", "1.", "1.0000001", "18446744073709551616"} {
		if _, err := ParseAmount(value, 6); err == nil {
			t.Fatalf("accepted %q", value)
		}
	}
}

func FuzzAmountRoundtrip(f *testing.F) {
	f.Add(uint64(0), uint8(9))
	f.Add(^uint64(0), uint8(6))
	f.Fuzz(func(t *testing.T, raw uint64, decimals uint8) {
		parsed, err := ParseAmount(FormatAmount(raw, decimals), decimals)
		if err != nil || parsed != raw {
			t.Fatalf("roundtrip %d/%d: %d %v", raw, decimals, parsed, err)
		}
	})
}
