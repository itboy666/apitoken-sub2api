package tronpayment

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func fixture(t *testing.T, file string) map[string]any {
	t.Helper()
	b, e := os.ReadFile("testdata/" + file + ".json")
	require.NoError(t, e)
	var d map[string]any
	require.NoError(t, json.Unmarshal(b, &d))
	return d
}
func row(d map[string]any) map[string]any { return d["data"].([]any)[0].(map[string]any) }
func event(d map[string]any) map[string]any {
	return d["trc20TransferInfo"].([]any)[0].(map[string]any)
}
func TestFindPaymentAuthenticity(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any, map[string]any)
		paid   bool
	}{
		{"real USDT", func(l, d map[string]any) {}, true},
		{"fake token same symbol", func(l, d map[string]any) {
			event(d)["contract_address"] = "TWrongContract"
			event(d)["symbol"] = "USDT"
		}, false},
		{"TRX", func(l, d map[string]any) { d["contractType"] = 1; delete(d, "trc20TransferInfo") }, false},
		{"forged discovery contract", func(l, d map[string]any) { row(l)["contract_address"] = "TWrongContract" }, false},
		{"wrong recipient", func(l, d map[string]any) { event(d)["to_address"] = event(d)["from_address"] }, false},
		{"wrong amount", func(l, d map[string]any) { event(d)["amount_str"] = "128000001" }, false},
		{"floating amount", func(l, d map[string]any) { event(d)["amount_str"] = 128000000 }, false},
		{"wrong decimals", func(l, d map[string]any) { event(d)["decimals"] = 18 }, false},
		{"unconfirmed", func(l, d map[string]any) { d["confirmed"] = false }, false},
		{"too few confirmations", func(l, d map[string]any) { d["confirmations"] = 19 }, false},
		{"missing confirmations", func(l, d map[string]any) { delete(d, "confirmations") }, false},
		{"failed receipt", func(l, d map[string]any) { d["contractRet"] = "FAILED" }, false},
		{"reverted", func(l, d map[string]any) { d["revert"] = true }, false},
		{"missing revert evidence", func(l, d map[string]any) { delete(d, "revert") }, false},
		{"Approval event", func(l, d map[string]any) { event(d)["type"] = "Approval" }, false},
		{"failed event", func(l, d map[string]any) { event(d)["status"] = 1 }, false},
		{"mismatched block", func(l, d map[string]any) { d["block"] = 1 }, false},
		{"mismatched hash", func(l, d map[string]any) { d["hash"] = strings.Repeat("b", 64) }, false},
		{"old transfer", func(l, d map[string]any) { row(l)["block_timestamp"] = 1 }, false},
		{"duplicate matching events", func(l, d map[string]any) { d["trc20TransferInfo"] = append(d["trc20TransferInfo"].([]any), event(d)) }, false},
		{"self transfer", func(l, d map[string]any) {
			row(l)["from"] = row(l)["to"]
			event(d)["from_address"] = event(d)["to_address"]
		}, false},
		{"token metadata absent", func(l, d map[string]any) { delete(l, "tokenInfo") }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, d := fixture(t, "list"), fixture(t, "detail")
			ts := int64(d["timestamp"].(float64))
			addr := row(l)["to"].(string)
			invoice := Invoice{OrderID: "test-order", Address: addr, AmountMicro: "128000000", CreatedAt: time.UnixMilli(ts - 1000), ExpiresAt: time.UnixMilli(ts + 1000)}
			tt.mutate(l, d)
			c, e := New(Config{WalletAddress: addr, TronscanAPIKey: "test-api-key-never-secret"})
			require.NoError(t, e)
			c.http.Transport = roundTripper(func(r *http.Request) (*http.Response, error) {
				require.Equal(t, "https", r.URL.Scheme)
				require.Equal(t, apiHost, r.URL.Host)
				require.Equal(t, "test-api-key-never-secret", r.Header.Get("TRON-PRO-API-KEY"))
				require.NotContains(t, r.URL.String(), "test-api-key")
				data := l
				if r.URL.Path == detailPath {
					data = d
				}
				b, _ := json.Marshal(data)
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(b)))}, nil
			})
			proof, e := c.FindPayment(context.Background(), invoice)
			if tt.paid {
				require.NoError(t, e)
				require.NotNil(t, proof)
			} else {
				require.Nil(t, proof)
			}
		})
	}
}
func TestVerifierRejectsMalformedAndRedirects(t *testing.T) {
	require.Error(t, rejectDuplicateKeys([]byte(`{"confirmed":true,"confirmed":false}`)))
	for _, amount := range []string{"0", "-1", "1e6", "01", "1.0", "NaN", strings.Repeat("9", 78)} {
		require.False(t, ValidMicroAmount(amount))
	}
	c, e := New(Config{WalletAddress: "TWwECvk3qmaXWvQd6FrGU5rK6AJ78dyFB9", TronscanAPIKey: "test-api-key-never-secret"})
	require.NoError(t, e)
	require.ErrorIs(t, c.http.CheckRedirect(&http.Request{}, nil), http.ErrUseLastResponse)
	c.http.Transport = roundTripper(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 302, Body: io.NopCloser(strings.NewReader("secret-api-body"))}, nil
	})
	e = c.Test(context.Background())
	require.Error(t, e)
	require.NotContains(t, e.Error(), "secret-api-body")
}
