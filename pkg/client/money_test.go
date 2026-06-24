package client

import "testing"

func TestMoney_MinorUnits(t *testing.T) {
	cases := []struct {
		name string
		m    *Money
		want int64
	}{
		{"nil money", nil, 0},
		{"empty currency_code", &Money{Amount: 100, CurrencyCode: "", MinorUnitConversionRate: 100}, 0},
		{"USD 48000.00", &Money{Amount: 48000.00, CurrencyCode: "USD", MinorUnitConversionRate: 100}, 4800000},
		{"USD 0.01", &Money{Amount: 0.01, CurrencyCode: "USD", MinorUnitConversionRate: 100}, 1},
		{"USD 0.005 rounds up", &Money{Amount: 0.005, CurrencyCode: "USD", MinorUnitConversionRate: 100}, 1},
		{"USD 0.004 rounds down", &Money{Amount: 0.004, CurrencyCode: "USD", MinorUnitConversionRate: 100}, 0},
		{"JPY 12345 (rate 1)", &Money{Amount: 12345, CurrencyCode: "JPY", MinorUnitConversionRate: 1}, 12345},
		{"USD -1.50", &Money{Amount: -1.50, CurrencyCode: "USD", MinorUnitConversionRate: 100}, -150},
		{"missing rate defaults to 100", &Money{Amount: 10.00, CurrencyCode: "USD", MinorUnitConversionRate: 0}, 1000},
		{"negative rate defaults to 100", &Money{Amount: 10.00, CurrencyCode: "USD", MinorUnitConversionRate: -1}, 1000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.m.MinorUnits()
			if got != tc.want {
				t.Fatalf("MinorUnits(%+v) = %d, want %d", tc.m, got, tc.want)
			}
		})
	}
}
