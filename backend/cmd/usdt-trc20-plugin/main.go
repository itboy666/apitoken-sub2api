// USDT collection plugin: read-only chain verification, no wallet signing.
package main

import (
	"context"
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/tronpayment"
	api "github.com/Wei-Shaw/sub2api/pkg/pluginapi/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"sync"
	"time"
)

const pluginID = "local.apitoken.usdt-trc20"

type server struct {
	api.UnimplementedTransportPluginServer
	mu     sync.RWMutex
	client *tronpayment.Client
}

func (s *server) GetInfo(context.Context, *api.GetInfoRequest) (*api.GetInfoResponse, error) {
	return &api.GetInfoResponse{PluginId: pluginID, PluginVersion: "1.0.1", ProtocolVersion: api.ProtocolVersion, TransportApiVersion: api.TransportAPIVersion, Capabilities: []string{"payment.usdt_trc20.v1"}}, nil
}
func (s *server) Health(context.Context, *api.HealthRequest) (*api.HealthResponse, error) {
	return &api.HealthResponse{Healthy: true, Message: "USDT-TRC20 verifier ready"}, nil
}
func (s *server) ValidateConfig(_ context.Context, r *api.ValidateConfigRequest) (*api.ValidateConfigResponse, error) {
	c, e := tronpayment.ParseConfig(r.ConfigJson)
	if e != nil {
		return &api.ValidateConfigResponse{Message: e.Error()}, nil
	}
	b, _ := json.Marshal(c)
	return &api.ValidateConfigResponse{Valid: true, NormalizedConfigJson: b}, nil
}
func (s *server) ApplyConfig(_ context.Context, r *api.ApplyConfigRequest) (*api.ApplyConfigResponse, error) {
	c, e := tronpayment.ParseConfig(r.ConfigJson)
	if e != nil {
		return &api.ApplyConfigResponse{Message: e.Error()}, nil
	}
	client, e := tronpayment.New(c)
	if e != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid configuration")
	}
	s.mu.Lock()
	s.client = client
	s.mu.Unlock()
	return &api.ApplyConfigResponse{Applied: true}, nil
}
func (s *server) TestConfig(ctx context.Context, r *api.TestConfigRequest) (*api.TestConfigResponse, error) {
	start := time.Now()
	c, e := tronpayment.ParseConfig(r.ConfigJson)
	if e == nil {
		var client *tronpayment.Client
		client, e = tronpayment.New(c)
		if e == nil {
			e = client.Test(ctx)
		}
	}
	message := "TronScan 连接正常；仅核验官方 USDT，20 次确认后到账。"
	if e != nil {
		message = e.Error()
	}
	return &api.TestConfigResponse{Success: e == nil, Message: message, LatencyMs: time.Since(start).Milliseconds()}, nil
}
func (s *server) VerifyPayment(ctx context.Context, r *api.VerifyPaymentRequest) (*api.VerifyPaymentResponse, error) {
	s.mu.RLock()
	c := s.client
	s.mu.RUnlock()
	if c == nil {
		return nil, status.Error(codes.FailedPrecondition, "payment plugin is not configured")
	}
	proof, e := c.FindPayment(ctx, tronpayment.Invoice{OrderID: r.OrderId, Address: r.Address, AmountMicro: r.AmountMicro, CreatedAt: time.UnixMilli(r.CreatedAtMs), ExpiresAt: time.UnixMilli(r.ExpiresAtMs)})
	if e != nil {
		return nil, status.Error(codes.Unavailable, e.Error())
	}
	if proof == nil {
		return &api.VerifyPaymentResponse{}, nil
	}
	return &api.VerifyPaymentResponse{Paid: true, TransactionHash: proof.TransactionHash, ContractAddress: proof.ContractAddress, ToAddress: proof.ToAddress, FromAddress: proof.FromAddress, AmountMicro: proof.AmountMicro, Block: proof.Block, TimestampMs: proof.Timestamp.UnixMilli(), Confirmations: proof.Confirmations}, nil
}
func main() { api.Serve(&server{}) }
