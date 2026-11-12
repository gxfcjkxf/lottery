package cash

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const USDTContract = "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"
const usdtHex = "a614f803b6fd780986a42c78ec9c7f77e6ded13c"
const transferTopic = "ddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"

type TronSource struct {
	BaseURL, Key string
	Client       *http.Client
}

func NewTronSource() *TronSource {
	return &TronSource{BaseURL: "https://api.trongrid.io", Key: os.Getenv("TRONGRID_API_KEY"), Client: &http.Client{Timeout: 15 * time.Second}}
}
func (s *TronSource) request(ctx context.Context, path string, body any, out any) error {
	var data io.Reader
	method := http.MethodGet
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		data = bytes.NewReader(raw)
		method = http.MethodPost
	}
	req, err := http.NewRequestWithContext(ctx, method, s.BaseURL+path, data)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if s.Key != "" {
		req.Header.Set("TRON-PRO-API-KEY", s.Key)
	}
	res, err := s.Client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("TRON query failed: HTTP %d", res.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(out)
}
func tronAddress(topic string) (string, error) {
	if len(topic) != 64 || strings.Trim(topic[:24], "0") != "" {
		return "", ErrInvalid
	}
	raw, err := hex.DecodeString("41" + topic[24:])
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(raw)
	h = sha256.Sum256(h[:])
	raw = append(raw, h[:4]...)
	n := new(big.Int).SetBytes(raw)
	base := big.NewInt(58)
	r := new(big.Int)
	var out []byte
	for n.Sign() > 0 {
		n.QuoRem(n, base, r)
		out = append(out, "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"[r.Int64()])
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return string(out), nil
}

type tronReceipt struct {
	ID        string `json:"id"`
	Result    string `json:"result"`
	Error     string `json:"Error"`
	Timestamp int64  `json:"blockTimeStamp"`
	Receipt   struct {
		Result string `json:"result"`
	} `json:"receipt"`
	Logs []struct {
		Address string   `json:"address"`
		Topics  []string `json:"topics"`
		Data    string   `json:"data"`
	} `json:"log"`
}

func (s *TronSource) Transfers(ctx context.Context, address string, since time.Time) ([]USDTTransfer, error) {
	q := url.Values{"only_confirmed": {"true"}, "only_to": {"true"}, "contract_address": {USDTContract}, "limit": {"200"}, "order_by": {"block_timestamp,asc"}, "min_timestamp": {strconv.FormatInt(since.UnixMilli(), 10)}}
	out := []USDTTransfer{}
	seen := map[string]bool{}
	fingerprints := map[string]bool{}
	for {
		var page struct {
			Success bool `json:"success"`
			Data    []struct {
				Transaction string `json:"transaction_id"`
			} `json:"data"`
			Meta struct {
				Fingerprint string `json:"fingerprint"`
				Links       struct {
					Next string `json:"next"`
				} `json:"links"`
			} `json:"meta"`
		}
		if err := s.request(ctx, "/v1/accounts/"+address+"/transactions/trc20?"+q.Encode(), nil, &page); err != nil {
			return nil, err
		}
		if !page.Success {
			return nil, fmt.Errorf("TRON history request unsuccessful")
		}
		for _, item := range page.Data {
			if !hashReference(item.Transaction) {
				return nil, ErrInvalid
			}
			if seen[item.Transaction] {
				continue
			}
			seen[item.Transaction] = true
			var receipt tronReceipt
			if err := s.request(ctx, "/walletsolidity/gettransactioninfobyid", map[string]string{"value": item.Transaction}, &receipt); err != nil {
				return nil, err
			}
			if receipt.Error != "" {
				return nil, fmt.Errorf("TRON receipt query unsuccessful")
			}
			if receipt.ID == "" {
				continue
			}
			if receipt.ID != item.Transaction {
				return nil, ErrInvalid
			}
			if receipt.Result == "FAILED" || receipt.Receipt.Result != "SUCCESS" {
				continue
			}
			transfers, err := receipt.transfers(address)
			if err != nil {
				return nil, err
			}
			out = append(out, transfers...)
		}
		if page.Meta.Links.Next == "" {
			break
		}
		fp := page.Meta.Fingerprint
		if fp == "" || fingerprints[fp] {
			return nil, fmt.Errorf("TRON invalid pagination")
		}
		fingerprints[fp] = true
		q.Set("fingerprint", fp)
	}
	return out, nil
}
func (r tronReceipt) transfers(address string) ([]USDTTransfer, error) {
	out := []USDTTransfer{}
	for i, log := range r.Logs {
		if log.Address != usdtHex || len(log.Topics) != 3 || log.Topics[0] != transferTopic {
			continue
		}
		to, e := tronAddress(log.Topics[2])
		if e != nil {
			return nil, e
		}
		if to != address {
			continue
		}
		from, e := tronAddress(log.Topics[1])
		if e != nil {
			return nil, e
		}
		amount, ok := new(big.Int).SetString(log.Data, 16)
		if !ok || len(log.Data) != 64 || !amount.IsInt64() || amount.Sign() <= 0 || r.Timestamp <= 0 {
			return nil, ErrInvalid
		}
		out = append(out, USDTTransfer{Transaction: r.ID, Index: i, Sender: from, Address: to, Amount: amount.Int64(), Time: time.UnixMilli(r.Timestamp)})
	}
	return out, nil
}
