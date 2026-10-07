package withdrawal

import "github.com/gxfcjkxf/lottery/backend/internal/points"

// TurnoverBaseSnapshot is the server-owned basis from the locked wallet before
// reservation. It does not decide how different N values or turnover combine.
type TurnoverBaseSnapshot struct {
	RechargeAvailable points.Amount `json:"recharge_available"`
	GiftAvailable     points.Amount `json:"gift_available"`
	Points            points.Amount `json:"points"`
	WalletVersion     int64         `json:"wallet_version,string"`
}

func turnoverBaseSnapshot(wallet points.Wallet) (TurnoverBaseSnapshot, error) {
	if wallet.Version < 0 {
		return TurnoverBaseSnapshot{}, points.ErrInvalid
	}
	// Validate all sixteen buckets and their total before adding this subset.
	// Nonnegative, bounded totals make the subset sum safe without float math.
	if err := wallet.BySource.Validate(); err != nil {
		return TurnoverBaseSnapshot{}, err
	}
	recharge, _ := points.SourceIndex("recharge")
	gift, _ := points.SourceIndex("gift")
	available, _ := points.StateIndex("available")
	r, g := wallet.BySource[recharge][available], wallet.BySource[gift][available]
	return TurnoverBaseSnapshot{RechargeAvailable: r, GiftAvailable: g, Points: r + g, WalletVersion: wallet.Version}, nil
}
