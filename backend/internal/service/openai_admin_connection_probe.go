package service

import (
	"context"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Only the explicit administrator IQ endpoint constructs this request-local
// intent. It never selects a production connection or changes account policy.
type openAIAdminConnectionProbeKey struct{}

type openAIAdminConnectionProbe struct {
	accountID      int64
	connectionID   string
	requestedModel string
	mappedModel    string
	expiresAt      time.Time
	parentAccount  *Account
	parentRoute    [32]byte
}

func adminConnectionProbeFromContext(ctx context.Context) (openAIAdminConnectionProbe, bool) {
	probe, ok := ctx.Value(openAIAdminConnectionProbeKey{}).(openAIAdminConnectionProbe)
	return probe, ok
}

func (p openAIAdminConnectionProbe) valid(ctx context.Context, account *Account) bool {
	return ctx.Err() == nil && time.Now().Before(p.expiresAt) && account != nil &&
		account.ID == p.accountID && account.Platform == PlatformOpenAI &&
		account.Type == AccountTypeOAuth && !account.IsCredentialShadow()
}

func (p openAIAdminConnectionProbe) matchesRequest(ctx context.Context, account *Account, model string) bool {
	return p.valid(ctx, account) && model == p.requestedModel &&
		account.GetMappedModel(model) == p.mappedModel
}

func (p openAIAdminConnectionProbe) matchesProjection(ctx context.Context, account *Account) bool {
	return p.valid(ctx, account) && account.AuthConnectionID == p.connectionID &&
		account.GetExtraString("openai_connection_kind") == "bps" &&
		account.GetCredential("client_id") == openai.ExcelClientID
}

func (p openAIAdminConnectionProbe) permitsConnection(connection *OpenAIAuthConnection) bool {
	return connection != nil && connection.ID == p.connectionID && connection.AccountID == p.accountID &&
		!connection.Legacy && connection.Kind == "bps" && connection.IdentityVerified &&
		connection.ClientID == openai.ExcelClientID && connection.Supports("responses")
}

func adminConnectionProbeUnavailable() error {
	return infraerrors.BadRequest("OPENAI_CONNECTION_PROBE_UNAVAILABLE", "The selected BPS connection is unavailable for this account, model, or test request; no other connection was used")
}

// Recheck the original parent after token refresh and proxy/slot waits, without
// selecting another connection or comparing its projected credentials to the
// parent. This runs at every physical send, including a BPS retry.
func (s *OpenAIGatewayService) fenceAdminConnectionProbeParent(ctx context.Context, projected *Account) error {
	probe, explicit := adminConnectionProbeFromContext(ctx)
	if !explicit {
		return nil
	}
	if !probe.matchesProjection(ctx, projected) || probe.parentAccount == nil {
		return adminConnectionProbeUnavailable()
	}
	ctx = context.WithValue(ctx, qualityProbeContextKey{}, false)
	latest, err := s.admitOpenAITurnWithGroup(ctx, probe.parentAccount, probe.requestedModel, 0, false)
	if err != nil {
		return err
	}
	if !probe.matchesRequest(ctx, latest, probe.requestedModel) || openAITurnRouteFingerprint(latest) != probe.parentRoute {
		return denyOpenAITurn("account_binding_changed")
	}
	return nil
}

// TestPelicanAuthConnection is called only by the authenticated administrator
// manual IQ handler. Scheduled tests and public gateway input cannot create it.
func (s *AccountTestService) TestPelicanAuthConnection(c *gin.Context, accountID int64, modelID, prompt, effort, connectionID string) error {
	connectionID = strings.TrimSpace(connectionID)
	if connectionID == "" {
		return s.TestPelicanAccountConnection(c, accountID, modelID, prompt, effort)
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Minute)
	defer cancel()
	c.Request = c.Request.WithContext(ctx)
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil || account == nil || s.openaiGatewayService == nil || s.openaiGatewayService.excelOAuthReauth == nil {
		return s.sendErrorAndEnd(c, adminConnectionProbeUnavailable().Error())
	}
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		modelID = openai.DefaultTestModel
	}
	probe := openAIAdminConnectionProbe{
		accountID: accountID, connectionID: connectionID,
		requestedModel: modelID, mappedModel: account.GetMappedModel(modelID),
		expiresAt:     time.Now().Add(5 * time.Minute),
		parentAccount: cloneOpenAITurnAdmissionAccount(account), parentRoute: openAITurnRouteFingerprint(account),
	}
	if !probe.valid(ctx, account) || account.IsSyntheticUITest() || isOpenAIImageModel(probe.mappedModel) ||
		!s.openaiGatewayService.excelBPSGloballyEnabled(ctx) {
		return s.sendErrorAndEnd(c, adminConnectionProbeUnavailable().Error())
	}
	connection, err := s.openaiGatewayService.excelOAuthReauth.authConnection(ctx, accountID, connectionID)
	if err != nil || !probe.permitsConnection(connection) {
		return s.sendErrorAndEnd(c, adminConnectionProbeUnavailable().Error())
	}
	ctx = logger.IntoContext(ctx, logger.FromContext(ctx).With(
		zap.String("auth_connection_id", connection.ID),
		zap.String("auth_connection_kind", connection.Kind),
		zap.String("test_route", "explicit_bps_connection"),
		zap.String("mapped_model", probe.mappedModel),
	))
	c.Request = c.Request.WithContext(context.WithValue(ctx, openAIAdminConnectionProbeKey{}, probe))
	return s.TestPelicanAccountConnection(c, accountID, modelID, prompt, effort)
}
