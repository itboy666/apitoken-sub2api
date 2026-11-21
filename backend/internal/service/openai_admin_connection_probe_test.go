package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const adminProbeConnectionID = "920ddc27-232c-4db7-9bf8-e1b061a1260b"

type adminProbeAccountRepo struct{ turnAdmissionRepo }

func (r *adminProbeAccountRepo) GetByID(context.Context, int64) (*Account, error) {
	return r.account, r.err
}

func adminProbeFixture(t *testing.T) (*AccountTestService, *Account, *connectionTypeRepo, *httpUpstreamRecorder) {
	t.Helper()
	reauth, connections := connectionTypeService(AccountTypeOAuth)
	parent := &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"access_token": "synthetic-native-token", "client_id": openai.ClientID, "chatgpt_account_id": "native-workspace"},
		Extra:       map[string]any{"openai_excel_bps": false, "openai_connections_managed": false}}
	reauth.accounts = &reauthTestAccountReader{account: parent}
	connection := &OpenAIAuthConnection{ID: adminProbeConnectionID, AccountID: 42, Kind: "bps", Label: "Independent BPS", Status: "active", RefreshState: "idle", Revision: 1,
		IdentityVerified: true, ClientID: openai.ExcelClientID, Capabilities: []string{"responses"}}
	require.NoError(t, reauth.encryptConnectionCredentials(connection, map[string]any{
		"access_token": "synthetic-bps-token", "client_id": openai.ExcelClientID, "chatgpt_account_id": "bps-workspace",
		"expires_at": strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10),
	}))
	connections.rows = []*OpenAIAuthConnection{connection}
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}, "X-Request-Id": {"req_probe_synthetic"}},
		Body: io.NopCloser(strings.NewReader("data: {\"type\":\"response.output_text.delta\",\"delta\":\"21\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_probe\",\"status\":\"completed\",\"model\":\"gpt-6.1-sol\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"21\"}]}],\"usage\":{\"input_tokens\":10,\"output_tokens\":2}}}\n\n"))}}
	gateway := openAIClientToolsTestService(upstream)
	accounts := &adminProbeAccountRepo{turnAdmissionRepo: turnAdmissionRepo{account: parent}}
	gateway.accountRepo, gateway.excelOAuthReauth = accounts, reauth
	gateway.requireLatestTurnAdmission = true
	return &AccountTestService{accountRepo: accounts, openaiGatewayService: gateway}, parent, connections, upstream
}

func adminProbeContext(parent *Account) context.Context {
	return context.WithValue(context.Background(), openAIAdminConnectionProbeKey{}, openAIAdminConnectionProbe{
		accountID: parent.ID, connectionID: adminProbeConnectionID, requestedModel: "gpt-6.1-sol", mappedModel: parent.GetMappedModel("gpt-6.1-sol"), expiresAt: time.Now().Add(time.Minute),
		parentAccount: cloneOpenAITurnAdmissionAccount(parent), parentRoute: openAITurnRouteFingerprint(parent),
	})
}

func TestAdminIQConnectionProbeRechecksParentAfterProjection(t *testing.T) {
	for _, change := range []string{"paused", "disabled", "expired", "model_isolation", "rate_limited", "model_mapping", "proxy_binding"} {
		t.Run(change, func(t *testing.T) {
			svc, parent, _, upstream := adminProbeFixture(t)
			gateway := svc.openaiGatewayService
			gateway.cfg.Gateway.Scheduling.OpenAITurnAdmissionCacheTTLSeconds = 300
			gateway.openAITurnAdmissionCache.store(parent, nil)
			ctx := adminProbeContext(parent)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
			projected, err := gateway.admitOpenAITurn(ctx, c, parent, "gpt-6.1-sol")
			require.NoError(t, err, "the selected grant is ready before the concurrent administrative change")
			// Simulate a committed update while token/proxy/slot work is in
			// progress. The projected grant and cached parent remain unchanged.
			latest := cloneOpenAITurnAdmissionAccount(parent)
			switch change {
			case "paused":
				latest.Schedulable = false
			case "disabled":
				latest.Status = "disabled"
			case "expired":
				past := time.Now().Add(-time.Minute)
				latest.ExpiresAt, latest.AutoPauseOnExpired = &past, true
			case "model_isolation":
				latest.Extra[QualityModelIsolationKey] = map[string]any{"gpt-6.1-sol": map[string]any{"reason": "quality_rule:7"}}
			case "rate_limited":
				latest.Extra[modelRateLimitsKey] = map[string]any{"gpt-6.1-sol": map[string]any{"rate_limit_reset_at": time.Now().Add(time.Hour).Format(time.RFC3339)}}
			case "model_mapping":
				latest.Credentials["model_mapping"] = map[string]any{"gpt-6.1-sol": "gpt-6-astra"}
			case "proxy_binding":
				proxyID := int64(27)
				latest.ProxyID = &proxyID
			}
			svc.accountRepo.(*adminProbeAccountRepo).account = latest
			cached, _, ok := gateway.openAITurnAdmissionCache.load(parent.ID, 300*time.Second)
			require.True(t, ok)
			require.True(t, cached.IsSchedulable(), "the old process cache alone would still admit the account")
			req := httptest.NewRequest(http.MethodPost, "https://bps.openai.com/basispoints/api/responses", strings.NewReader(`{"model":"gpt-6.1-sol"}`)).WithContext(ctx)
			_, err = gateway.doPriorityObservedHTTP(req, "", projected)
			require.Error(t, err)
			require.Empty(t, upstream.requests, "the final physical send must stop after the parent changes")
		})
	}
}

func TestAdminIQConnectionProbeUsesIndependentBPSWithoutSelecting(t *testing.T) {
	for _, managed := range []bool{false, true} {
		t.Run(strconv.FormatBool(managed), func(t *testing.T) {
			svc, parent, connections, upstream := adminProbeFixture(t)
			connections.managed = managed
			parent.Extra["openai_connections_managed"] = managed
			connections.rows = append(connections.rows, &OpenAIAuthConnection{
				ID: "9472ef89-962b-4ef6-8eb1-a3a2d6c51403", AccountID: parent.ID,
				Kind: "codex", Status: "active", Preferred: true, Capabilities: []string{"responses"},
			})
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/42/pelican-test", nil)
			c.Request.Header.Set("Cookie", "private-native-cookie")
			c.Request.Header.Set("X-Codex-Turn-State", "private-native-ticket")
			err := svc.TestPelicanAuthConnection(c, 42, "gpt-6.1-sol", "Return 21", "high", adminProbeConnectionID)
			require.NoError(t, err)
			require.Len(t, upstream.requests, 1)
			require.Equal(t, "bps.openai.com", upstream.lastReq.URL.Host)
			require.Equal(t, "Bearer synthetic-bps-token", upstream.lastReq.Header.Get("Authorization"))
			require.Equal(t, "bps-workspace", upstream.lastReq.Header.Get("Chatgpt-Account-Id"))
			require.Empty(t, upstream.lastReq.Header.Get("Cookie"))
			require.Empty(t, upstream.lastReq.Header.Get("X-Codex-Turn-State"))
			require.Equal(t, "high", gjson.GetBytes(upstream.lastBody, "reasoning_effort").String())
			require.Equal(t, "/api/v1/admin/accounts/42/pelican-test", c.Request.URL.Path)
			require.Empty(t, c.Request.Header.Get("Session-Id"))
			require.Contains(t, rec.Body.String(), `"type":"auth_connection"`)
			require.Contains(t, rec.Body.String(), `"connection_id":"`+adminProbeConnectionID+`"`)
			require.Contains(t, rec.Body.String(), `"type":"upstream_response"`)
			require.Contains(t, rec.Body.String(), `"upstream_status":200`)
			require.Contains(t, rec.Body.String(), `"success":true`)
			require.NotContains(t, rec.Body.String(), "synthetic-bps-token")
			require.Equal(t, 0, connections.writes)
			require.False(t, connections.rows[0].Preferred)
			require.True(t, connections.rows[1].Preferred)
			require.Equal(t, managed, connections.managed)
			require.False(t, parent.IsExcelBPSEnabled())
			require.Equal(t, "synthetic-native-token", parent.GetCredential("access_token"))
		})
	}
}

func TestAdminIQConnectionProbeRejectsUnavailableGrantWithoutSending(t *testing.T) {
	for _, change := range []string{"wrong_account", "wrong_id", "disabled", "pending", "cooldown", "no_responses", "wrong_client", "unverified", "codex", "shadow", "non_oauth", "invalid_id"} {
		t.Run(change, func(t *testing.T) {
			svc, parent, connections, upstream := adminProbeFixture(t)
			connection := connections.rows[0]
			id := adminProbeConnectionID
			switch change {
			case "wrong_account":
				connection.AccountID = 77
			case "wrong_id":
				id = "31b4c561-f9a0-49b4-a98a-99dd70ea6be5"
			case "disabled":
				connection.Status = "disabled"
			case "pending":
				connection.RefreshState = "pending"
			case "cooldown":
				until := time.Now().Add(time.Hour)
				connection.CooldownUntil = &until
			case "no_responses":
				connection.Capabilities = []string{"chat_completions"}
			case "wrong_client":
				connection.ClientID = openai.ClientID
			case "unverified":
				connection.IdentityVerified = false
			case "codex":
				connection.Kind = "codex"
			case "shadow":
				id := int64(22)
				parent.ParentAccountID = &id
			case "non_oauth":
				parent.Type = AccountTypeAPIKey
			case "invalid_id":
				id = "not-a-uuid"
			}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/admin/accounts/42/pelican-test", nil)
			require.Error(t, svc.TestPelicanAuthConnection(c, 42, "gpt-6.1-sol", "Return 21", "medium", id))
			require.Empty(t, upstream.requests)
			require.Equal(t, 0, connections.writes)
		})
	}
}

func TestAdminIQConnectionProbeKeepsAccountAdmission(t *testing.T) {
	for _, change := range []string{"paused", "disabled", "expired", "model_block", "rate_limited"} {
		t.Run(change, func(t *testing.T) {
			svc, parent, connections, upstream := adminProbeFixture(t)
			switch change {
			case "paused":
				parent.Schedulable = false
			case "disabled":
				parent.Status = "disabled"
			case "expired":
				past := time.Now().Add(-time.Minute)
				parent.ExpiresAt, parent.AutoPauseOnExpired = &past, true
			case "model_block":
				parent.Extra[QualityModelIsolationKey] = map[string]any{"gpt-6.1-sol": map[string]any{"reason": "quality_rule:7"}}
			case "rate_limited":
				parent.Extra[modelRateLimitsKey] = map[string]any{"gpt-6.1-sol": map[string]any{"rate_limit_reset_at": time.Now().Add(time.Hour).Format(time.RFC3339)}}
			}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/admin/accounts/42/pelican-test", nil)
			require.Error(t, svc.TestPelicanAuthConnection(c, 42, "gpt-6.1-sol", "Return 21", "medium", adminProbeConnectionID))
			require.Empty(t, upstream.requests)
			require.Equal(t, 0, connections.writes)
		})
	}
}

func TestAdminIQConnectionProbeFencesPolicyAndRevocation(t *testing.T) {
	svc, parent, connections, _ := adminProbeFixture(t)
	gateway := svc.openaiGatewayService
	ctx := adminProbeContext(parent)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
	// A saved production session must not override the explicit probe pin.
	c.Set("openai_auth_connection_session_hash", "existing-production-affinity")
	projected, err := gateway.prepareAuthConnection(ctx, c, parent, nil, "gpt-6.1-sol")
	require.NoError(t, err)
	require.Equal(t, adminProbeConnectionID, projected.AuthConnectionID)
	require.NoError(t, gateway.fenceAuthConnectionEgress(ctx, nil, projected))
	ordinary, err := gateway.prepareAuthConnection(context.Background(), c, parent, nil, "gpt-6.1-sol")
	require.NoError(t, err)
	require.Same(t, parent, ordinary, "the ordinary unmanaged route must remain on the parent")
	require.Error(t, gateway.fenceAuthConnectionEgress(context.Background(), nil, projected), "ordinary production cannot use an unselected managed grant")
	wrong := *projected
	wrong.ID++
	require.Error(t, gateway.fenceAuthConnectionEgress(ctx, nil, &wrong))
	wrong = *projected
	wrong.AuthConnectionID = "31b4c561-f9a0-49b4-a98a-99dd70ea6be5"
	require.Error(t, gateway.fenceAuthConnectionEgress(ctx, nil, &wrong))
	connections.rows[0].Revision++
	require.Error(t, gateway.fenceAuthConnectionEgress(ctx, nil, projected), "concurrent edit/revoke must still fence the already projected request")
	connections.rows[0].Revision--
	connections.rows[0].Status = "disabled"
	require.Error(t, gateway.fenceAuthConnectionEgress(ctx, nil, projected))
	connections.rows[0].Status = "active"
	connections.rows[0].Capabilities = []string{"chat_completions"}
	require.Error(t, gateway.fenceAuthConnectionEgress(ctx, nil, projected))
	connections.rows[0].Capabilities = []string{"responses"}
	connections.rows[0].ClientID = openai.ClientID
	require.Error(t, gateway.fenceAuthConnectionEgress(ctx, nil, projected))
	connections.managed = true
	require.Error(t, gateway.excelOAuthReauth.validateAuthConnectionPolicy(context.Background(), parent), "ordinary managed route still requires connection selection")
	require.Equal(t, 0, connections.writes)
}

func TestAdminIQConnectionProbeRejectsScopeReuse(t *testing.T) {
	for _, change := range []string{"path", "model", "mapping", "account", "deadline", "canceled"} {
		t.Run(change, func(t *testing.T) {
			svc, parent, _, upstream := adminProbeFixture(t)
			ctx := adminProbeContext(parent)
			path, model := "/v1/responses", "gpt-6.1-sol"
			switch change {
			case "path":
				path = "/admin/accounts/42/pelican-test"
			case "model":
				model = "gpt-6-astra"
			case "mapping":
				parent.Credentials["model_mapping"] = map[string]any{"gpt-6.1-sol": "gpt-6-astra"}
			case "account":
				parent.ID++
			case "deadline":
				probe, _ := adminConnectionProbeFromContext(ctx)
				probe.expiresAt = time.Now().Add(-time.Second)
				ctx = context.WithValue(ctx, openAIAdminConnectionProbeKey{}, probe)
			case "canceled":
				canceled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = canceled
			}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, path, nil).WithContext(ctx)
			body, err := buildExcelBPSAccountTestBody(model, "Return 21")
			require.NoError(t, err)
			_, err = svc.openaiGatewayService.Forward(ctx, c, parent, body)
			require.Error(t, err)
			require.Empty(t, upstream.requests)
		})
	}
}

func TestAdminIQConnectionProbeMapsModelOnce(t *testing.T) {
	svc, parent, _, upstream := adminProbeFixture(t)
	parent.Credentials["model_mapping"] = map[string]any{"requested-sol": "gpt-6.1-sol", "gpt-6.1-sol": "gpt-6-astra"}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/admin/accounts/42/pelican-test", nil)
	require.NoError(t, svc.TestPelicanAuthConnection(c, 42, "requested-sol", "Return 21", "medium", adminProbeConnectionID))
	require.Equal(t, "gpt-6.1-sol", gjson.GetBytes(upstream.lastBody, "model").String())
}
