/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package checker

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	verikubev1alpha1 "github.com/frauniki/verikube/api/v1alpha1"
)

const defaultGRPCTimeout = 5 * time.Second

// GRPCChecker observes whether a gRPC server reports SERVING via the
// standard gRPC Health Checking Protocol (grpc.health.v1.Health/Check).
type GRPCChecker struct {
	opts Options
}

func NewGRPCChecker(opts Options) *GRPCChecker {
	return &GRPCChecker{opts: opts}
}

func (g *GRPCChecker) Type() string { return TypeGRPC }

func (g *GRPCChecker) Check(ctx context.Context, spec verikubev1alpha1.CheckSpec) Result {
	cfg := spec.GRPC

	timeout := defaultGRPCTimeout
	if cfg.Timeout != nil {
		timeout = cfg.Timeout.Duration
	}

	creds := insecure.NewCredentials()
	if cfg.TLS != nil {
		creds = credentials.NewTLS(&tls.Config{
			InsecureSkipVerify: cfg.TLS.InsecureSkipVerify, // #nosec G402 -- user opt-in per check
			MinVersion:         tls.VersionTLS12,
		})
	}

	// The custom dialer routes the connection through the shared target
	// guard; gRPC's resolver has run by the time it is called, so the guard
	// sees the concrete IP.
	dialer := net.Dialer{Control: g.opts.dialControl()}
	conn, err := grpc.NewClient(cfg.Address,
		grpc.WithTransportCredentials(creds),
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, "tcp", addr)
		}),
	)
	if err != nil {
		return Result{Observed: false, Message: err.Error()}
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	resp, err := healthpb.NewHealthClient(conn).Check(ctx,
		&healthpb.HealthCheckRequest{Service: cfg.Service})
	elapsed := time.Since(start)
	if err != nil {
		return Result{Observed: false, Message: err.Error(), Duration: elapsed}
	}
	if resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		return Result{
			Observed: false,
			Message:  fmt.Sprintf("server reported health status %s, expected SERVING", resp.GetStatus()),
			Duration: elapsed,
		}
	}
	return Result{Observed: true, Duration: elapsed}
}
