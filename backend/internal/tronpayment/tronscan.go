// Package tronpayment verifies inbound mainnet USDt transfers through TronScan.
// It never signs transactions, holds a wallet key, or changes a user balance.
package tronpayment

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/btcsuite/btcd/btcutil/base58"
)

const (
	ContractAddress      = "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"
	Decimals             = 6
	MinimumConfirmations = 20
	apiHost              = "apilist.tronscanapi.com"
	listPath             = "/api/token_trc20/transfers-with-status"
	detailPath           = "/api/transaction-info"
	pageSize             = 50
	maxScanRecords       = 1000
	maxResponseBytes     = 4 << 20
)

var (
	hashPattern   = regexp.MustCompile(`^[0-9a-f]{64}$`)
	amountPattern = regexp.MustCompile(`^[1-9][0-9]{0,77}$`)
	keyPattern    = regexp.MustCompile(`^[A-Za-z0-9_-]{16,256}$`)
	ErrSchema     = errors.New("TronScan response schema is unavailable or invalid")
)

type Config struct {
	WalletAddress  string `json:"wallet_address"`
	TronscanAPIKey string `json:"tronscan_api_key"`
}

// ParseConfig rejects fields outside this read-only collection capability.
func ParseConfig(raw []byte) (Config, error) {
	var cfg Config
	if len(raw) == 0 || len(raw) > 4096 || rejectDuplicateKeys(raw) != nil {
		return cfg, errors.New("payment configuration must be a valid JSON object")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&cfg) != nil {
		return Config{}, errors.New("payment configuration fields are invalid")
	}
	cfg.WalletAddress = strings.TrimSpace(cfg.WalletAddress)
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if !ValidAddress(c.WalletAddress) {
		return errors.New("wallet_address must be a valid TRON mainnet Base58Check address")
	}
	if !keyPattern.MatchString(c.TronscanAPIKey) {
		return errors.New("tronscan_api_key is required and must be a valid API key")
	}
	return nil
}

func ValidAddress(address string) bool {
	if len(address) != 34 || address[0] != 'T' {
		return false
	}
	decoded := base58.Decode(address)
	if len(decoded) != 25 || decoded[0] != 0x41 {
		return false
	}
	first := sha256.Sum256(decoded[:21])
	second := sha256.Sum256(first[:])
	return bytes.Equal(decoded[21:], second[:4]) && base58.Encode(decoded) == address
}

// ValidMicroAmount accepts a canonical positive uint256 integer, never a
// decimal, float, signed value, exponential value, or formatted display amount.
func ValidMicroAmount(amount string) bool {
	if !amountPattern.MatchString(amount) {
		return false
	}
	value, ok := new(big.Int).SetString(amount, 10)
	return ok && value.Sign() > 0 && value.BitLen() <= 256
}

type Invoice struct {
	OrderID     string
	Address     string
	AmountMicro string
	CreatedAt   time.Time
	ExpiresAt   time.Time
}

func (i Invoice) Validate() error {
	if strings.TrimSpace(i.OrderID) == "" || len(i.OrderID) > 128 || !ValidAddress(i.Address) || !ValidMicroAmount(i.AmountMicro) {
		return errors.New("payment invoice identity or amount is invalid")
	}
	if i.CreatedAt.UnixMilli() <= 0 || !i.ExpiresAt.After(i.CreatedAt) || i.ExpiresAt.Sub(i.CreatedAt) > 24*time.Hour {
		return errors.New("payment invoice time window is invalid")
	}
	return nil
}

type Proof struct {
	TransactionHash string
	ContractAddress string
	ToAddress       string
	FromAddress     string
	AmountMicro     string
	Block           int64
	Timestamp       time.Time
	Confirmations   int64
}

type Client struct {
	config Config
	http   *http.Client
}

func New(cfg Config) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Client{config: cfg, http: &http.Client{
		Timeout: 15 * time.Second,
		// Redirects are forbidden even to the same host. The credential can only
		// ever accompany the original fixed HTTPS API request.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

// Test performs a read-only API/authentication and schema check. It makes no
// payment and does not claim historical transactions for any order.
func (c *Client) Test(ctx context.Context) error {
	now := time.Now().UTC()
	_, err := c.list(ctx, now.Add(-time.Hour), now, 0)
	return err
}

func (c *Client) request(ctx context.Context, path string, query url.Values) (map[string]json.RawMessage, error) {
	if path != listPath && path != detailPath {
		return nil, errors.New("unsupported TronScan operation")
	}
	endpoint := url.URL{Scheme: "https", Host: apiHost, Path: path, RawQuery: query.Encode()}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, errors.New("cannot create TronScan request")
	}
	req.Header.Set("TRON-PRO-API-KEY", c.config.TronscanAPIKey)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		// Never return upstream bodies, URLs, transport diagnostics or headers:
		// they can contain credentials and are not trustworthy UI messages.
		return nil, errors.New("TronScan is temporarily unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TronScan request failed (HTTP %d)", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil || len(body) > maxResponseBytes || rejectDuplicateKeys(body) != nil {
		return nil, ErrSchema
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(body, &object) != nil || object == nil {
		return nil, ErrSchema
	}
	return object, nil
}

type candidate struct {
	hash, from, to, amount string
	block, timestamp       int64
}

// FindPayment verifies candidates twice: the account's contract-specific list
// is discovery only; the independently fetched transaction detail is required
// evidence. The host must atomically claim the hash before fulfillment.
func (c *Client) FindPayment(ctx context.Context, invoice Invoice) (*Proof, error) {
	if err := invoice.Validate(); err != nil {
		return nil, err
	}
	if invoice.Address != c.config.WalletAddress {
		return nil, errors.New("invoice recipient does not match plugin configuration")
	}
	seen := make(map[string]bool)
	for start := 0; start < maxScanRecords; start += pageSize {
		rows, err := c.list(ctx, invoice.CreatedAt, invoice.ExpiresAt, start)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			candidate, ok := parseCandidate(row, invoice)
			if !ok || seen[candidate.hash] {
				continue
			}
			seen[candidate.hash] = true
			detail, err := c.request(ctx, detailPath, url.Values{"hash": {candidate.hash}})
			if err != nil {
				return nil, err
			}
			proof, err := verifyDetail(detail, candidate, invoice)
			if err != nil {
				return nil, err
			}
			if proof != nil {
				return proof, nil
			}
		}
		if len(rows) < pageSize {
			return nil, nil
		}
	}
	return nil, errors.New("TronScan transfer window exceeds the safe scan limit")
}

func (c *Client) list(ctx context.Context, from, to time.Time, offset int) ([]map[string]json.RawMessage, error) {
	result, err := c.request(ctx, listPath, url.Values{
		"trc20Id": {ContractAddress}, "address": {c.config.WalletAddress}, "direction": {"2"},
		"db_version": {"0"}, "reverse": {"false"}, "limit": {strconv.Itoa(pageSize)}, "start": {strconv.Itoa(offset)},
		"start_timestamp": {strconv.FormatInt(from.UnixMilli(), 10)}, "end_timestamp": {strconv.FormatInt(to.UnixMilli(), 10)},
	})
	if err != nil {
		return nil, err
	}
	code, codeOK := integer(result, "code")
	pageCount, countOK := integer(result, "page_size")
	var token map[string]json.RawMessage
	var rows []map[string]json.RawMessage
	if !codeOK || code != 200 || !countOK || pageCount < 0 || pageCount > pageSize ||
		json.Unmarshal(result["tokenInfo"], &token) != nil || token == nil ||
		json.Unmarshal(result["data"], &rows) != nil || rows == nil || int64(len(rows)) != pageCount {
		return nil, ErrSchema
	}
	decimals, decimalsOK := integer(token, "tokenDecimal")
	if text(token, "tokenId") != ContractAddress || !decimalsOK || decimals != Decimals || text(token, "tokenType") != "trc20" {
		return nil, ErrSchema
	}
	return rows, nil
}

func parseCandidate(row map[string]json.RawMessage, invoice Invoice) (candidate, bool) {
	item := candidate{hash: text(row, "hash"), from: text(row, "from"), to: text(row, "to"), amount: text(row, "amount")}
	block, blockOK := integer(row, "block")
	timestamp, timeOK := integer(row, "block_timestamp")
	status, statusOK := integer(row, "status")
	confirmed, confirmedOK := integer(row, "confirmed")
	revert, revertOK := integer(row, "revert")
	decimals, decimalsOK := integer(row, "decimals")
	direction, directionOK := integer(row, "direction")
	contractType, contractTypeOK := integer(row, "contractType")
	item.block, item.timestamp = block, timestamp
	return item, hashPattern.MatchString(item.hash) && ValidAddress(item.from) && item.from != item.to && item.to == invoice.Address &&
		ValidMicroAmount(item.amount) && item.amount == invoice.AmountMicro &&
		text(row, "contract_address") == ContractAddress && text(row, "id") == ContractAddress &&
		text(row, "event_type") == "Transfer" && text(row, "contract_type") == "TriggerSmartContract" &&
		text(row, "contract_ret") == "SUCCESS" && text(row, "final_result") == "SUCCESS" &&
		blockOK && block > 0 && timeOK && inWindow(timestamp, invoice) && statusOK && status == 0 &&
		confirmedOK && confirmed == 1 && revertOK && revert == 0 && decimalsOK && decimals == Decimals &&
		directionOK && direction == 2 && contractTypeOK && contractType == 31
}

func verifyDetail(detail map[string]json.RawMessage, candidate candidate, invoice Invoice) (*Proof, error) {
	block, blockOK := integer(detail, "block")
	timestamp, timeOK := integer(detail, "timestamp")
	confirmations, confirmationsOK := integer(detail, "confirmations")
	confirmed, confirmedOK := boolean(detail, "confirmed")
	reverted, revertedOK := boolean(detail, "revert")
	contractType, typeOK := integer(detail, "contractType")
	if !blockOK || !timeOK || !confirmationsOK || !confirmedOK || !revertedOK || !typeOK || text(detail, "hash") == "" || text(detail, "contractRet") == "" {
		return nil, ErrSchema
	}
	if text(detail, "hash") != candidate.hash || block <= 0 || block != candidate.block || timestamp != candidate.timestamp ||
		!inWindow(timestamp, invoice) || !confirmed || reverted || confirmations < MinimumConfirmations ||
		contractType != 31 || text(detail, "contractRet") != "SUCCESS" {
		return nil, nil
	}
	var transfers []map[string]json.RawMessage
	if json.Unmarshal(detail["trc20TransferInfo"], &transfers) != nil || transfers == nil {
		return nil, ErrSchema
	}
	matches := 0
	for _, event := range transfers {
		status, statusOK := integer(event, "status")
		decimals, decimalsOK := integer(event, "decimals")
		if text(event, "contract_address") != ContractAddress || !statusOK || status != 0 || !decimalsOK || decimals != Decimals || text(event, "type") != "Transfer" {
			continue
		}
		// The array itself is TRC20-specific. When either compatibility field
		// is supplied, it must agree. Symbols and names are never authority.
		if raw, ok := event["tokenType"]; ok && (len(raw) == 0 || text(event, "tokenType") != "trc20") {
			continue
		}
		if raw, ok := event["tokenType2"]; ok && (len(raw) == 0 || text(event, "tokenType2") != "trc20") {
			continue
		}
		amount := text(event, "amount_str")
		from, to := text(event, "from_address"), text(event, "to_address")
		if !ValidAddress(from) || from != candidate.from || to != invoice.Address || from == to || !ValidMicroAmount(amount) || amount != invoice.AmountMicro {
			continue
		}
		matches++
	}
	if matches != 1 {
		return nil, nil
	}
	return &Proof{TransactionHash: candidate.hash, ContractAddress: ContractAddress, ToAddress: invoice.Address,
		FromAddress: candidate.from, AmountMicro: invoice.AmountMicro, Block: block,
		Timestamp: time.UnixMilli(timestamp).UTC(), Confirmations: confirmations}, nil
}

func inWindow(timestamp int64, invoice Invoice) bool {
	return timestamp >= invoice.CreatedAt.UnixMilli() && timestamp < invoice.ExpiresAt.UnixMilli()
}

func text(object map[string]json.RawMessage, key string) string {
	var value string
	if json.Unmarshal(object[key], &value) != nil {
		return ""
	}
	return value
}

func integer(object map[string]json.RawMessage, key string) (int64, bool) {
	raw, exists := object[key]
	if !exists || bytes.Equal(raw, []byte("null")) {
		return 0, false
	}
	var value int64
	err := json.Unmarshal(raw, &value)
	return value, err == nil
}

func boolean(object map[string]json.RawMessage, key string) (bool, bool) {
	raw, exists := object[key]
	if !exists || bytes.Equal(raw, []byte("null")) {
		return false, false
	}
	var value bool
	err := json.Unmarshal(raw, &value)
	return value, err == nil
}

// rejectDuplicateKeys prevents a malformed upstream response from assigning
// different financial meanings to the same field in different parsers.
func rejectDuplicateKeys(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 32 {
			return ErrSchema
		}
		token, err := decoder.Token()
		if err != nil {
			return ErrSchema
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for decoder.More() {
				token, err := decoder.Token()
				key, ok := token.(string)
				if err != nil || !ok || seen[key] {
					return ErrSchema
				}
				seen[key] = true
				if err := walk(depth + 1); err != nil {
					return err
				}
			}
		case '[':
			for decoder.More() {
				if err := walk(depth + 1); err != nil {
					return err
				}
			}
		default:
			return ErrSchema
		}
		_, err = decoder.Token()
		return err
	}
	if walk(0) != nil {
		return ErrSchema
	}
	if _, err := decoder.Token(); err != io.EOF {
		return ErrSchema
	}
	return nil
}
