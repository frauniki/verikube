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
	"net"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	verikubev1alpha1 "github.com/frauniki/verikube/api/v1alpha1"
)

func grpcSpec(address string, mutate func(*verikubev1alpha1.GRPCCheck)) verikubev1alpha1.CheckSpec {
	g := &verikubev1alpha1.GRPCCheck{Address: address}
	if mutate != nil {
		mutate(g)
	}
	return verikubev1alpha1.CheckSpec{Name: "grpc-test", GRPC: g}
}

// startHealthServer runs a gRPC health server on a loopback port and
// returns its address and the health service handle for status control.
func startHealthServer(t *testing.T) (string, *health.Server) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	hs := health.NewServer()
	healthpb.RegisterHealthServer(srv, hs)
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(srv.Stop)
	return ln.Addr().String(), hs
}

func TestGRPCCheckerServing(t *testing.T) {
	addr, _ := startHealthServer(t)

	res := NewGRPCChecker(allowLocal).Check(context.Background(), grpcSpec(addr, nil))
	if !res.Observed {
		t.Fatalf("expected success, got: %s", res.Message)
	}
	if res.Duration <= 0 {
		t.Fatal("expected a positive duration")
	}
}

func TestGRPCCheckerNotServing(t *testing.T) {
	addr, hs := startHealthServer(t)
	hs.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)

	res := NewGRPCChecker(allowLocal).Check(context.Background(), grpcSpec(addr, nil))
	if res.Observed {
		t.Fatal("expected failure for NOT_SERVING")
	}
	want := "server reported health status NOT_SERVING, expected SERVING"
	if res.Message != want {
		t.Fatalf("message mismatch:\n got: %s\nwant: %s", res.Message, want)
	}
}

func TestGRPCCheckerNamedService(t *testing.T) {
	addr, hs := startHealthServer(t)
	hs.SetServingStatus("payments.v1.Payments", healthpb.HealthCheckResponse_SERVING)

	res := NewGRPCChecker(allowLocal).Check(context.Background(), grpcSpec(addr, func(g *verikubev1alpha1.GRPCCheck) {
		g.Service = "payments.v1.Payments"
	}))
	if !res.Observed {
		t.Fatalf("expected success for a registered service, got: %s", res.Message)
	}
}

func TestGRPCCheckerUnknownService(t *testing.T) {
	addr, _ := startHealthServer(t)

	res := NewGRPCChecker(allowLocal).Check(context.Background(), grpcSpec(addr, func(g *verikubev1alpha1.GRPCCheck) {
		g.Service = "no.such.Service"
	}))
	if res.Observed {
		t.Fatal("expected failure for an unknown service")
	}
	if !strings.Contains(res.Message, "NotFound") && !strings.Contains(res.Message, "unknown service") {
		t.Fatalf("expected a NOT_FOUND error message, got: %s", res.Message)
	}
}

func TestGRPCCheckerClosedPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	res := NewGRPCChecker(allowLocal).Check(context.Background(), grpcSpec(addr, func(g *verikubev1alpha1.GRPCCheck) {
		g.Timeout = &metav1.Duration{Duration: 2 * time.Second}
	}))
	if res.Observed {
		t.Fatal("expected failure against a closed port")
	}
	if res.Message == "" {
		t.Fatal("expected a failure message")
	}
}

func TestGRPCCheckerGuardBlocksLoopback(t *testing.T) {
	addr, _ := startHealthServer(t)

	res := NewGRPCChecker(Options{}).Check(context.Background(), grpcSpec(addr, func(g *verikubev1alpha1.GRPCCheck) {
		g.Timeout = &metav1.Duration{Duration: 2 * time.Second}
	}))
	if res.Observed {
		t.Fatal("expected loopback target to be blocked by default")
	}
	if !strings.Contains(res.Message, "blocked by default") {
		t.Fatalf("unexpected message: %s", res.Message)
	}
}

func TestGRPCCheckerPlainTCPServer(t *testing.T) {
	// A listener that never speaks HTTP/2: the health RPC must fail within
	// the check timeout instead of hanging.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	start := time.Now()
	res := NewGRPCChecker(allowLocal).Check(context.Background(), grpcSpec(ln.Addr().String(), func(g *verikubev1alpha1.GRPCCheck) {
		g.Timeout = &metav1.Duration{Duration: 500 * time.Millisecond}
	}))
	if res.Observed {
		t.Fatal("expected failure against a non-gRPC server")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("timeout was not honored, took %s", elapsed)
	}
}
