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
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"

	verikubev1alpha1 "github.com/frauniki/verikube/api/v1alpha1"
)

const defaultHTTPTimeout = 30 * time.Second

// HTTPChecker observes whether an HTTP request returns an expected status.
type HTTPChecker struct {
	opts Options
}

func NewHTTPChecker(opts Options) *HTTPChecker {
	return &HTTPChecker{opts: opts}
}

func (h *HTTPChecker) Type() string { return TypeHTTP }

func (h *HTTPChecker) Check(ctx context.Context, spec verikubev1alpha1.CheckSpec) Result {
	cfg := spec.HTTP

	timeout := defaultHTTPTimeout
	if cfg.Timeout != nil {
		timeout = cfg.Timeout.Duration
	}
	method := cfg.Method
	if method == "" {
		method = http.MethodGet
	}
	expected := cfg.ExpectedStatus
	if len(expected) == 0 {
		expected = []int32{http.StatusOK}
	}

	dialer := net.Dialer{Control: h.opts.dialControl()}
	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext: dialer.DialContext,
		},
	}
	defer client.CloseIdleConnections()

	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, method, cfg.URL, nil)
	if err != nil {
		return Result{Observed: false, Message: err.Error(), Duration: time.Since(start)}
	}
	for _, hdr := range cfg.Headers {
		// A Host header must be set on the request itself to override the
		// target host, e.g. when probing through a load balancer.
		if strings.EqualFold(hdr.Name, "Host") {
			req.Host = hdr.Value
			continue
		}
		req.Header.Set(hdr.Name, hdr.Value)
	}

	resp, err := client.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		return Result{Observed: false, Message: err.Error(), Duration: elapsed}
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
	}()

	if !slices.Contains(expected, int32(resp.StatusCode)) {
		return Result{
			Observed: false,
			Message:  fmt.Sprintf("unexpected status code: got %d, expected one of %v", resp.StatusCode, expected),
			Duration: elapsed,
		}
	}
	return Result{Observed: true, Duration: elapsed}
}
