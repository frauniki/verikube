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

package controller

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Labels set on runner Jobs and their pods.
const (
	labelCheckRun = "verikube.dev/checkrun"
	labelRunner   = "verikube.dev/runner"
	labelSuite    = "verikube.dev/suite"
)

// truncateName shortens a name to maxLen while keeping it unique by
// replacing the tail with a short hash of the full name. Kubernetes object
// names allow up to 253 characters but Job names are limited to 63, and
// label values to 63 as well.
func truncateName(name string, maxLen int) string {
	if len(name) <= maxLen {
		return name
	}
	sum := sha256.Sum256([]byte(name))
	suffix := hex.EncodeToString(sum[:])[:8]
	return fmt.Sprintf("%s-%s", name[:maxLen-9], suffix)
}

// jobName returns the Job name for a run's runner, kept within the 63
// character limit Jobs require (their pods get a generated suffix).
func jobName(runName, runnerName string) string {
	return truncateName(fmt.Sprintf("%s-%s", runName, runnerName), 57)
}

// hash8 returns a short stable hash, used to derive deterministic run names
// from manual trigger values.
func hash8(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:8]
}
