package business

import "testing"

func TestCatalogDecimalsRejectUnboundedAndNonDecimalInput(t *testing.T) {
	for _, value := range []string{"1e1001", "1/2", "0x10"} {
		t.Run(value, func(t *testing.T) {
			if normalizeDecimal(value) != nil {
				t.Fatal("invalid external decimal accepted")
			}
		})
	}
}

func TestMultiplierConversionRejectsUnboundedAndNonDecimalInput(t *testing.T) {
	for _, value := range []string{"1e1001", "1/2", "0x10"} {
		t.Run(value, func(t *testing.T) {
			if _, err := ConvertMultiplier(value, "1"); err == nil {
				t.Fatal("invalid source ratio accepted")
			}
			if _, err := ConvertMultiplier("1", value); err == nil {
				t.Fatal("invalid recharge ratio accepted")
			}
		})
	}
}

func TestManagementSnapshotDecimalsRejectUnsafeSyntax(t *testing.T) {
	for _, value := range []string{"1e1001", "1/2", "0x10"} {
		t.Run(value, func(t *testing.T) {
			if _, err := managementDecimal(value); err == nil {
				t.Error("invalid snapshot amount accepted")
			}
			if _, err := positiveQuotaUnit(value); err == nil {
				t.Fatal("invalid quota unit accepted")
			}
		})
	}
}
