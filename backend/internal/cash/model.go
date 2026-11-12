// Package cash owns member currency balances. All currency amounts use exact
// millionths internally; fiat inputs must be multiples of 0.01.
package cash

import (
	"crypto/sha256"
	"errors"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

var ErrInvalid = errors.New("invalid cash input")
var ErrDepositPending = errors.New("deposit already awaiting payment or verification")
var ErrConflict = errors.New("cash version or state changed")
var ErrUnavailable = errors.New("cash configuration unavailable")
var ErrInsufficient = errors.New("insufficient cash balance")
var ErrAccount = errors.New("incomplete payment account")
var ErrLimit = errors.New("cash limit exceeded")
var ErrRate = errors.New("exchange rate changed")
var Currencies = []string{"CNY", "USD", "THB", "USDT"}
var decimalPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]{1,6})?$`)

func Precision(currency string) (int, error) {
	switch currency {
	case "CNY", "USD", "THB":
		return 2, nil
	case "USDT":
		return 6, nil
	}
	return 0, ErrInvalid
}
func Parse(value, currency string) (int64, error) {
	p, e := Precision(currency)
	if e != nil || !decimalPattern.MatchString(value) {
		return 0, ErrInvalid
	}
	parts := strings.Split(value, ".")
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
		if len(fraction) > p {
			return 0, ErrInvalid
		}
	}
	n, ok := new(big.Int).SetString(parts[0]+fraction+strings.Repeat("0", 6-len(fraction)), 10)
	if !ok || !n.IsInt64() {
		return 0, ErrInvalid
	}
	return n.Int64(), nil
}
func Format(n int64) string {
	value := big.NewInt(n)
	negative := n < 0
	value.Abs(value)
	q, r := new(big.Int), new(big.Int)
	q.QuoRem(value, big.NewInt(1000000), r)
	s := q.String()
	if r.Sign() != 0 {
		s += "." + strings.TrimRight(strings.Repeat("0", 6-len(r.String()))+r.String(), "0")
	}
	if negative {
		s = "-" + s
	}
	return s
}

// Convert floors the destination amount to the destination's supported precision.
func Convert(amount int64, rate string, toPoints bool, currency string) (int64, error) {
	r, ok := new(big.Rat).SetString(rate)
	if !ok || r.Sign() <= 0 || !decimalPattern.MatchString(rate) {
		return 0, ErrInvalid
	}
	n := new(big.Rat).SetInt64(amount)
	if toPoints {
		n.Mul(n, r)
		n.Quo(n, big.NewRat(10000, 1))
	} else {
		n.Mul(n, big.NewRat(10000, 1))
		n.Quo(n, r)
	}
	out := new(big.Int).Quo(n.Num(), n.Denom())
	if !out.IsInt64() || out.Sign() <= 0 {
		return 0, ErrInvalid
	}
	v := out.Int64()
	if !toPoints {
		p, e := Precision(currency)
		if e != nil {
			return 0, e
		}
		if p == 2 {
			v = v / 10000 * 10000
		}
	}
	if v <= 0 {
		return 0, ErrInvalid
	}
	return v, nil
}

type Rule struct {
	Currency      string `json:"currency"`
	Rate          string `json:"rate"`
	DepositMin    string `json:"deposit_min"`
	DepositMax    string `json:"deposit_max"`
	WithdrawalMin string `json:"withdrawal_min"`
	WithdrawalMax string `json:"withdrawal_max"`
	Fee           string `json:"fee"`
	Mode          string `json:"mode"`
	Threshold     string `json:"threshold"`
	Enabled       bool   `json:"enabled"`
}
type Config struct {
	Version int64  `json:"version"`
	Rules   []Rule `json:"rules"`
}

func EmptyConfig() Config {
	c := Config{Rules: []Rule{}}
	for _, v := range Currencies {
		c.Rules = append(c.Rules, Rule{Currency: v, Rate: "", DepositMin: "0", DepositMax: "0", WithdrawalMin: "0", WithdrawalMax: "0", Fee: "0", Mode: "count", Threshold: "1"})
	}
	return c
}
func (c Config) Rule(currency string) (Rule, error) {
	for _, r := range c.Rules {
		if r.Currency == currency && r.Enabled {
			return r, nil
		}
	}
	return Rule{}, ErrUnavailable
}
func (c Config) Validate(platform bool) error {
	if len(c.Rules) != 4 || c.Version < 0 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, r := range c.Rules {
		if _, e := Precision(r.Currency); e != nil || seen[r.Currency] {
			return ErrInvalid
		}
		seen[r.Currency] = true
		a, e1 := Parse(r.DepositMin, r.Currency)
		b, e2 := Parse(r.DepositMax, r.Currency)
		d, e3 := Parse(r.WithdrawalMin, r.Currency)
		e, e4 := Parse(r.WithdrawalMax, r.Currency)
		f, e5 := Parse(r.Fee, r.Currency)
		if e1 != nil || e2 != nil || e3 != nil || e4 != nil || e5 != nil || a > b || d > e || r.Enabled && (a <= 0 || d <= 0 || f >= d) {
			return ErrInvalid
		}
		if platform {
			rate, ok := new(big.Rat).SetString(r.Rate)
			if r.Enabled && (!ok || rate.Sign() <= 0 || !decimalPattern.MatchString(r.Rate)) {
				return ErrInvalid
			}
		} else {
			if r.Mode == "count" {
				n, err := strconv.ParseInt(r.Threshold, 10, 64)
				if err != nil || n < 1 || strconv.FormatInt(n, 10) != r.Threshold {
					return ErrInvalid
				}
			} else if r.Mode == "amount" {
				n, e := Parse(r.Threshold, r.Currency)
				if e != nil || n <= 0 {
					return ErrInvalid
				}
			} else {
				return ErrInvalid
			}
		}
	}
	return nil
}

type Destination struct {
	ID          string `json:"id"`
	Currency    string `json:"currency"`
	Channel     string `json:"channel"`
	Holder      string `json:"holder"`
	Institution string `json:"institution"`
	Number      string `json:"number"`
	Enabled     bool   `json:"enabled"`
	Version     int64  `json:"version"`
}

func (d Destination) Validate() error {
	if _, e := Precision(d.Currency); e != nil {
		return e
	}
	for _, s := range []string{d.Channel, d.Holder, d.Institution, d.Number} {
		if strings.TrimSpace(s) != s || len(s) > 200 || strings.ContainsAny(s, "\n\r\x00") {
			return ErrAccount
		}
	}
	if d.Currency == "USDT" {
		if d.Channel != "TRON/TRC20" || !ValidTRON(d.Number) {
			return ErrAccount
		}
	} else if d.Channel != "bank" || d.Holder == "" || d.Institution == "" || d.Number == "" {
		return ErrAccount
	}
	return nil
}
func ValidTRON(address string) bool {
	const alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	if len(address) != 34 || address[0] != 'T' {
		return false
	}
	n := new(big.Int)
	for _, r := range address {
		i := strings.IndexRune(alphabet, r)
		if i < 0 {
			return false
		}
		n.Mul(n, big.NewInt(58))
		n.Add(n, big.NewInt(int64(i)))
	}
	b := n.Bytes()
	if len(b) != 25 || b[0] != 0x41 {
		return false
	}
	h := sha256.Sum256(b[:21])
	h = sha256.Sum256(h[:])
	return string(h[:4]) == string(b[21:])
}

type Balance struct {
	Currency     string `json:"currency"`
	Deposited    string `json:"deposited"`
	Withdrawable string `json:"withdrawable"`
	Reserved     string `json:"reserved"`
}
type Order struct {
	CreditedAt      string      `json:"credited_at"`
	Sender          string      `json:"sender"`
	Automatic       bool        `json:"automatic"`
	RequestedAmount string      `json:"requested_amount"`
	MemberAccount   string      `json:"member_account"`
	ID              string      `json:"id"`
	BrandID         string      `json:"brand_id"`
	MemberID        string      `json:"member_id"`
	Kind            string      `json:"kind"`
	Currency        string      `json:"currency"`
	Amount          string      `json:"amount"`
	Points          string      `json:"points"`
	Fee             string      `json:"fee"`
	Rate            string      `json:"rate"`
	State           string      `json:"state"`
	Version         int64       `json:"version"`
	Destination     Destination `json:"destination"`
	Proof           string      `json:"proof"`
	Reference       string      `json:"reference"`
	CreatedAt       string      `json:"created_at"`
	ExpiresAt       string      `json:"expires_at"`
}
type Input struct {
	Fee                string `json:"fee"`
	DestinationVersion int64  `json:"destination_version"`
	Currency           string `json:"currency"`
	Amount             string `json:"amount"`
	Rate               string `json:"rate"`
	Direction          string `json:"direction"`
	DestinationID      string `json:"destination_id"`
	Version            int64  `json:"version"`
	Proof              string `json:"proof"`
	Reference          string `json:"reference"`
}
