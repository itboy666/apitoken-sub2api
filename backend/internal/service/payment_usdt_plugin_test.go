package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"fmt"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/tronpayment"
	"github.com/Wei-Shaw/sub2api/migrations"
	api "github.com/Wei-Shaw/sub2api/pkg/pluginapi/v1"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestUSDTExactCNYConversion(t *testing.T) {
	for _, tt := range []struct {
		cny   float64
		micro int64
	}{{6.5, 1000000}, {65, 10000000}, {10, 1540000}, {0.01, 10000}} {
		n, e := usdtBaseMicro(tt.cny)
		require.NoError(t, e)
		require.Equal(t, tt.micro, n)
	}
	_, e := usdtBaseMicro(-1)
	require.Error(t, e)
}
func TestUSDTSecretMaskAndPreserve(t *testing.T) {
	raw := json.RawMessage(`{"wallet_address":"wallet","tronscan_api_key":"secret-key-value"}`)
	masked, e := maskPaymentPluginConfig(raw)
	require.NoError(t, e)
	require.NotContains(t, string(masked), "secret-key-value")
	require.Contains(t, string(masked), `"api_key_set":true`)
	merged, e := mergePaymentPluginSecret(masked, raw)
	require.NoError(t, e)
	require.Contains(t, string(merged), "secret-key-value")
	require.NotContains(t, string(merged), "api_key_set")
}
func TestUSDTPostgresClaims(t *testing.T) {
	dsn := os.Getenv("USDT_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set USDT_TEST_POSTGRES_DSN to a disposable PostgreSQL database")
	}
	ctx := context.Background()
	db, e := sql.Open("pgx", dsn)
	require.NoError(t, e)
	defer db.Close()
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	require.NoError(t, client.Schema.Create(ctx))
	for _, name := range []string{"229_plugins.sql", "388_usdt_payment_plugin.sql", "389_usdt_active_amount_reservations.sql"} {
		b, e := migrations.FS.ReadFile(name)
		require.NoError(t, e)
		_, e = db.ExecContext(ctx, string(b))
		require.NoError(t, e)
	}
	// Dedicated disposable DB only. All created rows belong to this test.
	suffix := fmt.Sprint(time.Now().UnixNano())
	hashFor := func(label string) string { b := sha256.Sum256([]byte(suffix + label)); return hex.EncodeToString(b[:]) }
	u, e := client.User.Create().SetEmail("usdt-" + suffix + "@example.test").SetPasswordHash("test").SetUsername("usdt-test").Save(ctx)
	require.NoError(t, e)
	var pluginID int64
	e = db.QueryRowContext(ctx, `INSERT INTO sub2api_plugin_installations(plugin_key,name,version,artifact_path,install_path,binary_path,binary_sha256,state) VALUES($1,'test','1.0.0','','','','hash','enabled') RETURNING id`, "test.usdt."+suffix).Scan(&pluginID)
	require.NoError(t, e)
	_, e = db.ExecContext(ctx, `UPDATE sub2api_plugin_bindings SET enabled=FALSE WHERE capability=$1`, PluginCapabilityUSDTTRC20)
	require.NoError(t, e)
	_, e = db.ExecContext(ctx, `INSERT INTO sub2api_plugin_bindings(plugin_id,capability,platform,account_type,enabled) VALUES($1,$2,'payment','none',TRUE)`, pluginID, PluginCapabilityUSDTTRC20)
	require.NoError(t, e)
	svc := &PaymentService{entClient: client}
	makeOrder := func(state string) (*dbent.PaymentOrder, *usdtInvoice) {
		o, e := client.PaymentOrder.Create().SetUserID(u.ID).SetUserEmail(u.Email).SetUserName(u.Username).SetAmount(65).SetPayAmount(65).SetFeeRate(0).SetRechargeCode("test-" + generateOutTradeNo()).SetOutTradeNo(generateOutTradeNo()).SetPaymentType(PaymentTypeUSDTPlugin).SetProviderKey(PaymentTypeUSDTPlugin).SetPaymentTradeNo("").SetOrderType(payment.OrderTypeBalance).SetStatus(state).SetExpiresAt(time.Now().Add(time.Minute)).SetClientIP("127.0.0.1").SetSrcHost("example.test").Save(ctx)
		require.NoError(t, e)
		tx, e := client.Tx(ctx)
		require.NoError(t, e)
		e = reserveUSDTInvoice(ctx, tx, o, CreateOrderRequest{usdtPluginID: pluginID, usdtWallet: "TWwECvk3qmaXWvQd6FrGU5rK6AJ78dyFB9"})
		require.NoError(t, e)
		require.NoError(t, tx.Commit())
		i, e := svc.usdtInvoice(ctx, o.ID)
		require.NoError(t, e)
		return o, i
	}
	proof := func(i *usdtInvoice, hash string) *api.VerifyPaymentResponse {
		return &api.VerifyPaymentResponse{Paid: true, TransactionHash: hash, ContractAddress: tronpayment.ContractAddress, ToAddress: i.Wallet, FromAddress: "TFBu1Z7SdGyWxLrKyov8GuKttoJRU3xKox", AmountMicro: i.AmountMicro, Block: 1, TimestampMs: i.CreatedAt.UnixMilli(), Confirmations: 20}
	}
	o, i := makeOrder(OrderStatusPending)
	p := proof(i, hashFor("a"))
	require.Error(t, svc.requireUSDTClaim(ctx, o, p.TransactionHash, 65))
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for n := 0; n < 12; n++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- svc.claimUSDTProof(ctx, i, p) }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		require.NoError(t, e)
	}
	// A new service/process reads the durable claim. One hash cannot credit two orders.
	restarted := &PaymentService{entClient: client}
	require.NoError(t, restarted.requireUSDTClaim(ctx, o, p.TransactionHash, 65))
	require.Error(t, restarted.requireUSDTClaim(ctx, o, p.TransactionHash, 64.99))
	_, second := makeOrder(OrderStatusPending)
	require.NotEqual(t, i.AmountMicro, second.AmountMicro)
	require.Error(t, svc.claimUSDTProof(ctx, second, proof(second, p.TransactionHash)))
	_, cancelled := makeOrder(OrderStatusCancelled)
	require.Error(t, svc.claimUSDTProof(ctx, cancelled, proof(cancelled, hashFor("c"))))
	_, e = client.PaymentOrder.UpdateOneID(o.ID).SetStatus(OrderStatusCancelled).Save(ctx)
	require.NoError(t, e)
	require.NoError(t, svc.toPaid(ctx, o, p.TransactionHash, 65, PaymentTypeUSDTPlugin))
	current, e := client.PaymentOrder.Get(ctx, o.ID)
	require.NoError(t, e)
	require.Equal(t, OrderStatusCancelled, current.Status)
	require.Nil(t, current.PaidAt)
	require.Error(t, svc.confirmPayment(ctx, o.ID, p.TransactionHash, 65, PaymentTypeUSDTPlugin, nil))
	_, exp := makeOrder(OrderStatusExpired)
	require.NoError(t, svc.claimUSDTProof(ctx, exp, proof(exp, hashFor("e"))))
	p2 := proof(second, hashFor("b"))
	p2.ContractAddress = "fake"
	require.Error(t, svc.claimUSDTProof(ctx, second, p2))
	_, e = db.ExecContext(ctx, `UPDATE sub2api_plugin_bindings SET enabled=FALSE WHERE plugin_id=$1`, pluginID)
	require.NoError(t, e)
	require.Error(t, svc.claimUSDTProof(ctx, second, proof(second, hashFor("b"))))
}

func TestUSDTNativeRuntime(t *testing.T) {
	path := os.Getenv("USDT_PLUGIN_BINARY")
	if path == "" {
		t.Skip("set USDT_PLUGIN_BINARY to the built payment plugin")
	}
	path, e := filepath.Abs(path)
	require.NoError(t, e)
	b, e := os.ReadFile(path)
	require.NoError(t, e)
	hash := sha256.Sum256(b)
	inst := &PluginInstallation{PluginKey: "local.apitoken.usdt-trc20", Version: "1.0.1", BinaryPath: path, BinarySHA256: hex.EncodeToString(hash[:])}
	socket, e := os.MkdirTemp("/tmp", "usdt-sock-")
	require.NoError(t, e)
	defer os.RemoveAll(socket)
	rt, e := startPluginRuntime(t.Context(), inst, 5*time.Second, socket, nil)
	require.NoError(t, e)
	defer rt.kill()
	raw := json.RawMessage(`{"wallet_address":"TWwECvk3qmaXWvQd6FrGU5rK6AJ78dyFB9","tronscan_api_key":"fixture-api-key-not-secret"}`)
	require.NoError(t, rt.validateAndApplyConfig(t.Context(), raw))
	require.NoError(t, rt.checkHealth(t.Context()))
	scope := pluginAccountScopeFromManifest(PluginManifest{Capabilities: []PluginCapability{{ID: PluginCapabilityUSDTTRC20, Platform: "payment", AccountType: "none"}}})
	require.True(t, scope.Empty())
	invalid, e := rt.api.ValidateConfig(t.Context(), &api.ValidateConfigRequest{ConfigJson: []byte(`{"wallet_address":"bad"}`)})
	require.NoError(t, e)
	require.False(t, invalid.Valid)
}

func TestUSDTSignedPackageLifecycle(t *testing.T) {
	for _, version := range []string{"2.23.0-apitoken.6", "2.23.0-apitoken.7"} {
		t.Run(version, func(t *testing.T) { testUSDTSignedPackageLifecycle(t, version) })
	}
}

func testUSDTSignedPackageLifecycle(t *testing.T, version string) {
	path := os.Getenv("USDT_PLUGIN_PACKAGE")
	if path == "" {
		t.Skip("set USDT_PLUGIN_PACKAGE to the signed payment package")
	}
	raw, e := os.ReadFile(path)
	require.NoError(t, e)
	m, baseRepo, _ := websiteTestManager(t)
	repo := &usdtMemoryRepo{websiteMemoryRepo: baseRepo}
	short, err := os.MkdirTemp("/tmp", "usdt-test-")
	require.NoError(t, err)
	defer os.RemoveAll(short)
	m = NewPluginManager(repo, m.encryptor, testPluginConfig(short, false), PluginHostInfo{Version: version}, nil)
	defer m.Stop()
	p, e := m.Install(t.Context(), bytes.NewReader(raw), nil)
	require.NoError(t, e)
	require.True(t, p.Manifest.IsPaymentPlugin())
	require.Equal(t, PluginSignatureTrusted, p.SignatureStatus)
	cfg := json.RawMessage(`{"wallet_address":"TWwECvk3qmaXWvQd6FrGU5rK6AJ78dyFB9","tronscan_api_key":"fixture-api-key-not-secret"}`)
	masked, e := m.SaveConfig(t.Context(), p.ID, cfg)
	require.NoError(t, e)
	require.NotContains(t, string(masked), "fixture-api-key")
	_, e = m.Enable(t.Context(), p.ID, false, 100)
	require.NoError(t, e)
	require.Nil(t, m.route.Load())
	require.NoError(t, m.reconcileOnce(t.Context()))
	_, _, e = m.activePaymentPlugin(t.Context())
	require.NoError(t, e)
	m.Stop()
	restarted := NewPluginManager(repo, m.encryptor, m.cfg, m.hostInfo, nil)
	defer restarted.Stop()
	require.NoError(t, restarted.reconcileOnce(t.Context()))
	_, _, e = restarted.activePaymentPlugin(t.Context())
	require.NoError(t, e)
	require.Nil(t, restarted.route.Load())
	_, e = restarted.Disable(t.Context(), p.ID)
	require.NoError(t, e)
	_, _, e = restarted.activePaymentPlugin(t.Context())
	require.Error(t, e)
	require.NoError(t, restarted.Delete(t.Context(), p.ID))
	require.Empty(t, trustedPluginPublisherKey(nil, "apitoken-usdt-trc20-v1", "malicious.other-plugin"))
}

type usdtMemoryRepo struct{ *websiteMemoryRepo }

func (r *usdtMemoryRepo) MarkRuntimeHealthy(ctx context.Context, id int64, hash, encrypted string) error {
	p, err := r.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if p.BinarySHA256 != hash || p.ConfigEncrypted != encrypted || !hasEnabledPluginBinding(p.Bindings) {
		return ErrPluginStateChanged
	}
	now := time.Now()
	return r.UpdateState(ctx, id, PluginStateEnabled, "", &now, hash, p.State)
}

func TestUSDTAmountDisplayPreservesIssuedInvoices(t *testing.T) {
	require.Equal(t, "1.60", formatUSDTMicro(decimal.NewFromInt(1600000)))
	require.Equal(t, "1.600408", formatUSDTMicro(decimal.NewFromInt(1600408)))
}

func TestUSDTPostgresAmountReservations(t *testing.T) {
	dsn := os.Getenv("USDT_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("disposable PostgreSQL required")
	}
	ctx := context.Background()
	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	defer db.Close()
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	for _, name := range []string{"388_usdt_payment_plugin.sql", "389_usdt_active_amount_reservations.sql"} {
		migration, err := migrations.FS.ReadFile(name)
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, string(migration))
		require.NoError(t, err)
	}
	id := time.Now().UnixMicro()
	// The allocator receives an already validated wallet from the plugin manager.
	wallet := fmt.Sprintf("T%033d", id)
	defer db.ExecContext(ctx, `DELETE FROM sub2api_usdt_invoices WHERE wallet=$1`, wallet)
	defer db.ExecContext(ctx, `DELETE FROM sub2api_usdt_amount_reservations WHERE wallet=$1`, wallet)
	created := time.Now().UTC().Truncate(time.Millisecond)
	makeInvoice := func(n int, start time.Time, offset int, rollback bool) (int64, error) {
		tx, err := client.Tx(ctx)
		if err != nil {
			return 0, err
		}
		defer tx.Rollback()
		order := &dbent.PaymentOrder{ID: id + int64(n), OutTradeNo: fmt.Sprintf("allocation-%d-%d", id, n), PayAmount: 6.5, CreatedAt: start, ExpiresAt: start.Add(30 * time.Minute)}
		err = reserveUSDTInvoiceWithOffset(ctx, tx, order, CreateOrderRequest{usdtPluginID: 1, usdtWallet: wallet}, offset)
		if err != nil {
			return 0, err
		}
		rows, err := tx.Client().QueryContext(ctx, `SELECT amount_micro FROM sub2api_usdt_invoices WHERE order_id=$1`, order.ID)
		if err != nil {
			return 0, err
		}
		var amount int64
		if rows.Next() {
			err = rows.Scan(&amount)
		}
		rows.Close()
		if err != nil {
			return 0, err
		}
		if rollback {
			return amount, tx.Rollback()
		}
		return amount, tx.Commit()
	}
	// Cross-connection concurrent requests all start at 1.03 and get distinct cents.
	type result struct {
		amount int64
		err    error
	}
	results := make(chan result, 8)
	for n := 0; n < 8; n++ {
		go func(n int) { amount, err := makeInvoice(n, created, 3, false); results <- result{amount, err} }(n)
	}
	amounts := []int64{}
	for n := 0; n < 8; n++ {
		result := <-results
		require.NoError(t, result.err)
		amounts = append(amounts, result.amount)
	}
	sort.Slice(amounts, func(i, j int) bool { return amounts[i] < amounts[j] })
	for n, amount := range amounts {
		require.Equal(t, int64(1_030_000+n*10_000), amount)
	}
	// Overlapping intervals remain occupied, even if allocation waits past expiry.
	amount, err := makeInvoice(20, created.Add(29*time.Minute), 3, false)
	require.NoError(t, err)
	require.Equal(t, int64(1_110_000), amount)
	// An expired interval can be reused; the old transaction timestamp cannot match it.
	later := created.Add(30*time.Minute + time.Millisecond)
	amount, err = makeInvoice(21, later, 3, false)
	require.NoError(t, err)
	require.Equal(t, int64(1_030_000), amount)
	invoice := &usdtInvoice{Wallet: wallet, AmountMicro: strconv.FormatInt(amount, 10), CreatedAt: later, ExpiresAt: later.Add(time.Minute)}
	require.False(t, validUSDTProof(invoice, &api.VerifyPaymentResponse{Paid: true, TransactionHash: fmt.Sprintf("%064d", 1), ContractAddress: tronpayment.ContractAddress, ToAddress: wallet, FromAddress: "TFBu1Z7SdGyWxLrKyov8GuKttoJRU3xKox", AmountMicro: invoice.AmountMicro, Block: 1, Confirmations: 20, TimestampMs: created.UnixMilli()}))
	// Rolling back the enclosing transaction releases its amount as well.
	after := created.Add(2 * time.Hour)
	a, err := makeInvoice(30, after, 0, true)
	require.NoError(t, err)
	b, err := makeInvoice(31, after, 0, false)
	require.NoError(t, err)
	require.Equal(t, a, b)
}
