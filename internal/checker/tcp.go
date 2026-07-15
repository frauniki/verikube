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
	"time"

	"net"

	verikubev1alpha1 "github.com/frauniki/verikube/api/v1alpha1"
)

const defaultTCPTimeout = 1 * time.Second

// TCPChecker observes whether a TCP connection can be established.
type TCPChecker struct {
	opts Options
}

func NewTCPChecker(opts Options) *TCPChecker {
	return &TCPChecker{opts: opts}
}

func (t *TCPChecker) Type() string { return TypeTCP }

func (t *TCPChecker) Check(ctx context.Context, spec verikubev1alpha1.CheckSpec) Result {
	timeout := defaultTCPTimeout
	if spec.TCP.Timeout != nil {
		timeout = spec.TCP.Timeout.Duration
	}
	dialer := net.Dialer{
		Timeout: timeout,
		Control: t.opts.dialControl(),
	}

	start := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", spec.TCP.Address)
	elapsed := time.Since(start)
	if err != nil {
		return Result{Observed: false, Message: err.Error(), Duration: elapsed}
	}
	_ = conn.Close()
	return Result{Observed: true, Duration: elapsed}
}
