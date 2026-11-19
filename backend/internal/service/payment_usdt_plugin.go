package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/tronpayment"
	api "github.com/Wei-Shaw/sub2api/pkg/pluginapi/v1"
	"github.com/shopspring/decimal"
	"math/rand/v2"
	"regexp"
	"time"
)

const PaymentTypeUSDTPlugin = "usdt_trc20_plugin"
const usdtRecoveryWindow = 3 * time.Hour

var usdtHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type usdtInvoice struct {
	OrderID, PluginID               int64
	OutTradeNo, Wallet, AmountMicro string
	PayCNY                          string
	CreatedAt, ExpiresAt            time.Time
	Hash                            sql.NullString
	CheckFailed                     bool
}

func usdtBaseMicro(cny float64) (int64, error) {
	if !isValidProviderAmount(cny) {
		return 0, errors.New("invalid CNY amount")
	}
	n := decimal.NewFromFloat(cny).Div(decimal.NewFromFloat(6.5)).Shift(2).Ceil().Shift(4)
	if !n.IsPositive() || n.GreaterThan(decimal.NewFromInt(1_000_000_000_000_000)) {
		return 0, errors.New("USDT amount out of range")
	}
	return n.IntPart(), nil
}

// The reservation and invoice commit with the original order. Amounts remain
// occupied through the original expiry, even when cancelled or already paid.
// Reuse never overlaps transfer time windows; the global tx hash remains unique.
func reserveUSDTInvoice(ctx context.Context, tx *dbent.Tx, o *dbent.PaymentOrder, req CreateOrderRequest) error {
	return reserveUSDTInvoiceWithOffset(ctx, tx, o, req, rand.IntN(10))
}

func reserveUSDTInvoiceWithOffset(ctx context.Context, tx *dbent.Tx, o *dbent.PaymentOrder, req CreateOrderRequest, randomCents int) error {
	base, err := usdtBaseMicro(o.PayAmount)
	if err != nil {
		return err
	}
	if randomCents < 0 || randomCents > 9 {
		return errors.New("invalid USDT random offset")
	}
	for attempt := 0; attempt < 1000; attempt++ {
		micro := base + int64(randomCents+attempt)*10_000
		result, e := tx.Client().ExecContext(ctx, `INSERT INTO sub2api_usdt_amount_reservations
          (wallet,contract,amount_micro,reserved_until) VALUES($1,$2,$3,$4)
          ON CONFLICT(wallet,contract,amount_micro) DO UPDATE SET reserved_until=EXCLUDED.reserved_until
          WHERE sub2api_usdt_amount_reservations.reserved_until <= $5`,
			req.usdtWallet, tronpayment.ContractAddress, micro, o.ExpiresAt, o.CreatedAt)
		if e != nil {
			return e
		}
		n, e := result.RowsAffected()
		if e != nil {
			return e
		}
		if n == 0 {
			continue
		}
		_, e = tx.Client().ExecContext(ctx, `INSERT INTO sub2api_usdt_invoices
          (order_id,out_trade_no,plugin_id,wallet,contract,amount_micro,pay_cny,rate_cny,created_at,expires_at)
          VALUES($1,$2,$3,$4,$5,$6,$7,6.5,$8,$9)`,
			o.ID, o.OutTradeNo, req.usdtPluginID, req.usdtWallet, tronpayment.ContractAddress, micro, o.PayAmount, o.CreatedAt, o.ExpiresAt)
		return e
	}
	return infraerrors.ServiceUnavailable("USDT_AMOUNT_UNAVAILABLE", "当前相近付款金额均被占用，请稍后重试或换一个充值金额")
}

func (s *PaymentService) createUSDTOrder(ctx context.Context, req CreateOrderRequest, user *User, plan *dbent.SubscriptionPlan, cfg *PaymentConfig, amount, limit, fee, pay, bonus float64) (*CreateOrderResponse, error) {
	p, c, err := s.configService.pluginManager.activePaymentPlugin(ctx)
	if err != nil {
		return nil, infraerrors.ServiceUnavailable("USDT_UNAVAILABLE", err.Error())
	}
	req.usdtPluginID = p.ID
	req.usdtWallet = c.WalletAddress
	// Invoice time windows are bounded by the verifier.
	localCfg := *cfg
	if localCfg.OrderTimeoutMin < 2 {
		localCfg.OrderTimeoutMin = 30
	}
	if localCfg.OrderTimeoutMin > 1440 {
		localCfg.OrderTimeoutMin = 1440
	}
	o, err := s.createOrderInTx(ctx, req, user, plan, &localCfg, amount, limit, fee, pay, bonus, nil)
	if err != nil {
		return nil, err
	}
	return &CreateOrderResponse{OrderID: o.ID, Amount: o.Amount, PayAmount: o.PayAmount, FeeRate: o.FeeRate, BonusAmount: o.BonusAmount,
		Status: o.Status, ResultType: payment.CreatePaymentResultOrderCreated, PaymentType: PaymentTypeUSDTPlugin, OutTradeNo: o.OutTradeNo,
		PayURL: "/payment/crypto/" + o.OutTradeNo, Currency: "CNY", ExpiresAt: o.ExpiresAt, PaymentMode: "redirect"}, nil
}

func (s *PaymentService) usdtInvoice(ctx context.Context, id int64) (*usdtInvoice, error) {
	rows, err := s.entClient.QueryContext(ctx, `SELECT order_id,plugin_id,out_trade_no,wallet,amount_micro::text,pay_cny::text,created_at,expires_at,transaction_hash,check_failed FROM sub2api_usdt_invoices WHERE order_id=$1`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, errors.New("USDT invoice not found")
	}
	i := &usdtInvoice{}
	err = rows.Scan(&i.OrderID, &i.PluginID, &i.OutTradeNo, &i.Wallet, &i.AmountMicro, &i.PayCNY, &i.CreatedAt, &i.ExpiresAt, &i.Hash, &i.CheckFailed)
	return i, err
}

func validUSDTProof(i *usdtInvoice, p *api.VerifyPaymentResponse) bool {
	return i != nil && p != nil && p.Paid && usdtHashPattern.MatchString(p.TransactionHash) && p.ContractAddress == tronpayment.ContractAddress &&
		p.ToAddress == i.Wallet && tronpayment.ValidAddress(p.FromAddress) && p.FromAddress != p.ToAddress && p.AmountMicro == i.AmountMicro &&
		p.Block > 0 && p.Confirmations >= tronpayment.MinimumConfirmations && p.TimestampMs >= i.CreatedAt.UnixMilli() && p.TimestampMs < i.ExpiresAt.UnixMilli() && p.TimestampMs <= time.Now().UnixMilli()
}

// Both the order and enabled plugin binding are locked when the global tx hash
// is claimed. Unique constraints arbitrate concurrent workers/processes.
func (s *PaymentService) claimUSDTProof(ctx context.Context, i *usdtInvoice, p *api.VerifyPaymentResponse) error {
	if !validUSDTProof(i, p) {
		return errors.New("invalid USDT payment proof")
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.Client().QueryContext(ctx, `SELECT status FROM payment_orders WHERE id=$1 AND payment_type=$2 AND pay_amount=$3 AND expires_at >= NOW()-INTERVAL '3 hours' FOR UPDATE`, i.OrderID, PaymentTypeUSDTPlugin, i.PayCNY)
	if err != nil {
		return err
	}
	var state string
	if rows.Next() {
		err = rows.Scan(&state)
	}
	rows.Close()
	if err != nil {
		return err
	}
	if state != OrderStatusPending && state != OrderStatusExpired {
		return errors.New("USDT order is no longer payable")
	}
	rows, err = tx.Client().QueryContext(ctx, `SELECT p.id FROM sub2api_plugin_installations p JOIN sub2api_plugin_bindings b ON b.plugin_id=p.id WHERE p.id=$1 AND p.state='enabled' AND b.enabled AND b.capability=$2 FOR SHARE OF p,b`, i.PluginID, PluginCapabilityUSDTTRC20)
	if err != nil {
		return err
	}
	enabled := rows.Next()
	rows.Close()
	if !enabled {
		return errors.New("USDT plugin is disabled")
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	result, err := tx.Client().ExecContext(ctx, `UPDATE sub2api_usdt_invoices SET transaction_hash=$1,proof=$2,claimed_at=NOW(),check_failed=FALSE WHERE order_id=$3 AND (transaction_hash IS NULL OR transaction_hash=$1)`, p.TransactionHash, string(raw), i.OrderID)
	if err != nil {
		return errors.New("USDT transaction has already been claimed or cannot be persisted")
	}
	n, err := result.RowsAffected()
	if err != nil || n != 1 {
		return errors.New("USDT invoice already has a different transaction")
	}
	return tx.Commit()
}

func (s *PaymentService) requireUSDTClaim(ctx context.Context, o *dbent.PaymentOrder, hash string, paid float64) error {
	i, err := s.usdtInvoice(ctx, o.ID)
	if err != nil {
		return err
	}
	expected, e := decimal.NewFromString(i.PayCNY)
	if e != nil || !i.Hash.Valid || i.Hash.String != hash || !usdtHashPattern.MatchString(hash) || !decimal.NewFromFloat(paid).Equal(expected) || o.OutTradeNo != i.OutTradeNo {
		return errors.New("USDT fulfillment requires a persisted verified transaction")
	}
	return nil
}

func (s *PaymentService) checkUSDTPaid(ctx context.Context, o *dbent.PaymentOrder) string {
	if o.PaymentType != PaymentTypeUSDTPlugin {
		return ""
	}
	if o.PaidAt != nil && (o.Status == OrderStatusPaid || o.Status == OrderStatusRecharging || o.Status == OrderStatusFailed) {
		_, _ = s.entClient.ExecContext(ctx, `UPDATE sub2api_usdt_invoices SET last_checked_at=NOW() WHERE order_id=$1`, o.ID)
		if s.requireUSDTClaim(ctx, o, o.PaymentTradeNo, o.PayAmount) == nil {
			_ = s.executeFulfillment(ctx, o.ID)
			return checkPaidResultAlreadyPaid
		}
		return ""
	}
	if (o.Status != OrderStatusPending && o.Status != OrderStatusExpired) || time.Now().After(o.ExpiresAt.Add(usdtRecoveryWindow)) {
		return ""
	}
	i, err := s.usdtInvoice(ctx, o.ID)
	if err != nil {
		return ""
	}
	if !i.Hash.Valid {
		// Distributed polling throttle; invoice rows are the source of truth.
		result, e := s.entClient.ExecContext(ctx, `UPDATE sub2api_usdt_invoices SET last_checked_at=NOW() WHERE order_id=$1 AND (last_checked_at IS NULL OR last_checked_at<NOW()-INTERVAL '15 seconds')`, o.ID)
		if e != nil {
			return ""
		}
		n, e := result.RowsAffected()
		if e != nil || n != 1 {
			return ""
		}
		proof, e := s.configService.pluginManager.verifyPluginPayment(ctx, i.PluginID, &api.VerifyPaymentRequest{OrderId: i.OutTradeNo, Address: i.Wallet, AmountMicro: i.AmountMicro, CreatedAtMs: i.CreatedAt.UnixMilli(), ExpiresAtMs: i.ExpiresAt.UnixMilli()})
		_, _ = s.entClient.ExecContext(ctx, `UPDATE sub2api_usdt_invoices SET check_failed=$1 WHERE order_id=$2`, e != nil, o.ID)
		if e != nil || proof == nil || !proof.Paid {
			return ""
		}
		if s.claimUSDTProof(ctx, i, proof) != nil {
			return ""
		}
		i.Hash = sql.NullString{String: proof.TransactionHash, Valid: true}
	}
	if err = s.toPaid(ctx, o, i.Hash.String, o.PayAmount, PaymentTypeUSDTPlugin); err != nil {
		return ""
	}
	current, err := s.entClient.PaymentOrder.Get(ctx, o.ID)
	if err == nil && current.PaidAt != nil {
		return checkPaidResultAlreadyPaid
	}
	return ""
}

func (s *PaymentService) ReconcileUSDTPaymentOrders(ctx context.Context) (int, error) {
	if s.configService == nil || s.configService.pluginManager == nil {
		return 0, nil
	}
	rows, err := s.entClient.QueryContext(ctx, `SELECT o.id FROM payment_orders o JOIN sub2api_usdt_invoices i ON i.order_id=o.id
 WHERE (o.status IN ('PENDING','EXPIRED') AND o.expires_at>=NOW()-INTERVAL '3 hours') OR (o.status IN ('PAID','RECHARGING','FAILED') AND o.paid_at IS NOT NULL)
 ORDER BY i.last_checked_at NULLS FIRST,o.id LIMIT 20`)
	if err != nil {
		return 0, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			break
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err != nil {
		return 0, err
	}
	count := 0
	for _, id := range ids {
		if ctx.Err() != nil {
			break
		}
		o, e := s.entClient.PaymentOrder.Get(ctx, id)
		if e == nil && s.checkUSDTPaid(ctx, o) == checkPaidResultAlreadyPaid {
			count++
		}
	}
	return count, nil
}

func (s *PaymentService) usdtCheckout(ctx context.Context, o *dbent.PaymentOrder) (*CryptoCheckout, error) {
	i, err := s.usdtInvoice(ctx, o.ID)
	if err != nil {
		return nil, err
	}
	amount, err := decimal.NewFromString(i.AmountMicro)
	if err != nil {
		return nil, err
	}
	network := "healthy"
	if i.CheckFailed {
		network = "delayed"
	}
	if _, _, err = s.configService.pluginManager.activePaymentPlugin(ctx); err != nil {
		network = "delayed"
	}
	return &CryptoCheckout{OrderID: o.ID, OutTradeNo: o.OutTradeNo, Status: o.Status, Network: "usdt.trc20", Crypto: "USDT", Address: i.Wallet, CryptoAmount: formatUSDTMicro(amount), FiatAmount: i.PayCNY, Currency: "CNY", ExpiresAt: i.ExpiresAt, TransactionHash: i.Hash.String, NetworkStatus: network, ExchangeRate: "6.5", ContractAddress: tronpayment.ContractAddress}, nil
}

// Keep already-issued six-decimal invoices exact. New invoices use cents.
func formatUSDTMicro(amount decimal.Decimal) string {
	if amount.Mod(decimal.NewFromInt(10_000)).IsZero() {
		return amount.Shift(-6).StringFixed(2)
	}
	return amount.Shift(-6).StringFixed(6)
}
