package cash

import "testing"

func TestWithdrawalFee(t *testing.T) {
	for _, tc := range []struct {
		setting, amount, currency string
		want                      int64
	}{
		{"50", "1000", "CNY", 50000000}, {"0", "1000", "USDT", 0}, {"10%", "123.45", "CNY", 12340000}, {"10%", "123.456789", "USDT", 12345678}, {"0.125%", "100", "USD", 120000}, {"99.999999%", "100", "USDT", 99999999},
	} {
		got, err := WithdrawalFee(tc.setting, tc.amount, tc.currency)
		if err != nil || got != tc.want {
			t.Fatalf("%+v: %d %v", tc, got, err)
		}
	}
	for _, setting := range []string{"100%", "101%", "-1", "10%%", "abc", "1.0000001%"} {
		if _, err := WithdrawalFee(setting, "100", "CNY"); err == nil {
			t.Fatalf("accepted %s", setting)
		}
	}
}
