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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	verikubev1alpha1 "github.com/frauniki/verikube/api/v1alpha1"
)

var allowLocal = Options{AllowLocalTargets: true}

func tcpSpec(address string, timeout *metav1.Duration) verikubev1alpha1.CheckSpec {
	return verikubev1alpha1.CheckSpec{
		Name: "tcp-test",
		TCP:  &verikubev1alpha1.TCPCheck{Address: address, Timeout: timeout},
	}
}

func httpSpec(mutate func(*verikubev1alpha1.HTTPCheck)) verikubev1alpha1.CheckSpec {
	h := &verikubev1alpha1.HTTPCheck{}
	if mutate != nil {
		mutate(h)
	}
	return verikubev1alpha1.CheckSpec{Name: "http-test", HTTP: h}
}

func TestTCPCheckerSuccess(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()

	res := NewTCPChecker(allowLocal).Check(context.Background(), tcpSpec(ln.Addr().String(), nil))
	if !res.Observed {
		t.Fatalf("expected success, got failure: %s", res.Message)
	}
	if res.Duration <= 0 {
		t.Fatal("expected a positive duration")
	}
}

func TestTCPCheckerClosedPort(t *testing.T) {
	// Grab a free port then close it so the dial is refused.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	res := NewTCPChecker(allowLocal).Check(context.Background(), tcpSpec(addr, nil))
	if res.Observed {
		t.Fatal("expected failure against a closed port")
	}
	if res.Message == "" {
		t.Fatal("expected a failure message")
	}
}

func TestTCPCheckerTimeout(t *testing.T) {
	// RFC 5737 TEST-NET-1 address: unroutable, so the dial hangs until timeout.
	timeout := &metav1.Duration{Duration: 50 * time.Millisecond}
	start := time.Now()
	res := NewTCPChecker(allowLocal).Check(context.Background(), tcpSpec("192.0.2.1:80", timeout))
	if res.Observed {
		t.Fatal("expected timeout failure")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("timeout was not honored, took %s", elapsed)
	}
}

func TestGuardBlocksLoopbackByDefault(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()

	res := NewTCPChecker(Options{}).Check(context.Background(), tcpSpec(ln.Addr().String(), nil))
	if res.Observed {
		t.Fatal("expected loopback target to be blocked by default")
	}
	if !strings.Contains(res.Message, "blocked by default") {
		t.Fatalf("unexpected message: %s", res.Message)
	}
}

func TestGuardBlocksLinkLocal(t *testing.T) {
	timeout := &metav1.Duration{Duration: 100 * time.Millisecond}
	res := NewTCPChecker(Options{}).Check(context.Background(), tcpSpec("169.254.169.254:80", timeout))
	if res.Observed {
		t.Fatal("expected link-local target to be blocked")
	}
	if !strings.Contains(res.Message, "blocked by default") {
		t.Fatalf("unexpected message: %s", res.Message)
	}
}

func TestHTTPCheckerDefaults(t *testing.T) {
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	res := NewHTTPChecker(allowLocal).Check(context.Background(), httpSpec(func(h *verikubev1alpha1.HTTPCheck) {
		h.URL = srv.URL
	}))
	if !res.Observed {
		t.Fatalf("expected success, got: %s", res.Message)
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("expected default method GET, got %s", gotMethod)
	}
}

func TestHTTPCheckerUnexpectedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	res := NewHTTPChecker(allowLocal).Check(context.Background(), httpSpec(func(h *verikubev1alpha1.HTTPCheck) {
		h.URL = srv.URL
	}))
	if res.Observed {
		t.Fatal("expected failure for 503")
	}
	want := "unexpected status code: got 503, expected one of [200]"
	if res.Message != want {
		t.Fatalf("message mismatch:\n got: %s\nwant: %s", res.Message, want)
	}
}

func TestHTTPCheckerExpectedStatusList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	res := NewHTTPChecker(allowLocal).Check(context.Background(), httpSpec(func(h *verikubev1alpha1.HTTPCheck) {
		h.URL = srv.URL
		h.ExpectedStatus = []int32{200, 202}
	}))
	if !res.Observed {
		t.Fatalf("expected 202 to be accepted, got: %s", res.Message)
	}
}

func TestHTTPCheckerHeadersAndHostOverride(t *testing.T) {
	var gotHost, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	res := NewHTTPChecker(allowLocal).Check(context.Background(), httpSpec(func(h *verikubev1alpha1.HTTPCheck) {
		h.URL = srv.URL
		h.Headers = []verikubev1alpha1.HTTPHeader{
			{Name: "Host", Value: "api.example.com"},
			{Name: "Authorization", Value: "Bearer token"},
		}
	}))
	if !res.Observed {
		t.Fatalf("expected success, got: %s", res.Message)
	}
	if gotHost != "api.example.com" {
		t.Fatalf("Host header not applied to request host, got %s", gotHost)
	}
	if gotAuth != "Bearer token" {
		t.Fatalf("Authorization header not sent, got %q", gotAuth)
	}
}

func TestHTTPCheckerTimeout(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release
		w.WriteHeader(http.StatusOK)
	}))
	defer func() {
		close(release)
		srv.Close()
	}()

	res := NewHTTPChecker(allowLocal).Check(context.Background(), httpSpec(func(h *verikubev1alpha1.HTTPCheck) {
		h.URL = srv.URL
		h.Timeout = &metav1.Duration{Duration: 50 * time.Millisecond}
	}))
	if res.Observed {
		t.Fatal("expected timeout failure")
	}
}

func TestHTTPCheckerMethod(t *testing.T) {
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	res := NewHTTPChecker(allowLocal).Check(context.Background(), httpSpec(func(h *verikubev1alpha1.HTTPCheck) {
		h.URL = srv.URL
		h.Method = "POST"
		h.ExpectedStatus = []int32{204}
	}))
	if !res.Observed {
		t.Fatalf("expected success, got: %s", res.Message)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("expected POST, got %s", gotMethod)
	}
}

func TestRegistryDispatch(t *testing.T) {
	reg := NewRegistry(NewTCPChecker(allowLocal), NewHTTPChecker(allowLocal), NewGRPCChecker(allowLocal))

	if c, ok := reg.ForSpec(tcpSpec("example.com:443", nil)); !ok || c.Type() != TypeTCP {
		t.Fatal("expected tcp checker for tcp spec")
	}
	if c, ok := reg.ForSpec(httpSpec(func(h *verikubev1alpha1.HTTPCheck) { h.URL = "https://example.com" })); !ok || c.Type() != TypeHTTP {
		t.Fatal("expected http checker for http spec")
	}
	if c, ok := reg.ForSpec(grpcSpec("example.com:50051", nil)); !ok || c.Type() != TypeGRPC {
		t.Fatal("expected grpc checker for grpc spec")
	}
	if _, ok := reg.ForSpec(verikubev1alpha1.CheckSpec{Name: "empty"}); ok {
		t.Fatal("expected no checker for a spec with no probe set")
	}

	// A registry missing the http checker must report unknown for http specs.
	tcpOnly := NewRegistry(NewTCPChecker(allowLocal))
	if _, ok := tcpOnly.ForSpec(httpSpec(func(h *verikubev1alpha1.HTTPCheck) { h.URL = "https://example.com" })); ok {
		t.Fatal("expected no checker when http is not registered")
	}
}
