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
	"fmt"
	"net"
	"syscall"
)

// Options configures probe behavior shared by all checkers.
type Options struct {
	// AllowLocalTargets permits probing loopback and link-local addresses.
	// Disabled by default so checks cannot reach cloud metadata endpoints
	// (169.254.169.254) or the runner pod itself.
	AllowLocalTargets bool
}

// dialControl returns a net.Dialer Control function enforcing the target
// guard. Control runs after DNS resolution with the concrete IP, so the
// guard also covers DNS names resolving to blocked ranges.
func (o Options) dialControl() func(network, address string, c syscall.RawConn) error {
	if o.AllowLocalTargets {
		return nil
	}
	return func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return fmt.Errorf("invalid target address %q: %w", address, err)
		}
		ip := net.ParseIP(host)
		if ip == nil {
			return fmt.Errorf("target %q did not resolve to an IP", address)
		}
		if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("target %s is a loopback or link-local address, blocked by default (enable allowLocalTargets to permit)", ip)
		}
		return nil
	}
}
