package pricing

import "testing"

func TestPriceAndRevenueDecimalsRejectUnboundedAndNonDecimalInput(t *testing.T) {
	for _, value := range []string{"1e1001", "1/2", "0x10"} {
		t.Run(value, func(t *testing.T) {
			if _, ok := positiveRat(value); ok {
				t.Error("invalid price accepted")
			}
			if _, ok := revenueAmount(value); ok {
				t.Error("invalid amount accepted")
			}
			if _, ok := positiveRevenueDecimal(value); ok {
				t.Error("invalid recharge ratio accepted")
			}
		})
	}
}
