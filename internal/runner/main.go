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

package runner

import (
	"flag"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	verikubev1alpha1 "github.com/frauniki/verikube/api/v1alpha1"
	"github.com/frauniki/verikube/internal/checker"
)

// Main is the entrypoint for the "runner" subcommand. It returns the
// process exit code: 0 when all checks were executed and reported, 1 on a
// runner error.
func Main(args []string) int {
	fs := flag.NewFlagSet("runner", flag.ExitOnError)
	maxConcurrent := fs.Int("max-concurrent-checks", 8,
		"Maximum number of checks executed concurrently.")
	allowLocal := fs.Bool("allow-local-targets", false,
		"Allow probing loopback and link-local targets (blocked by default).")
	zapOpts := zap.Options{}
	zapOpts.BindFlags(fs)
	_ = fs.Parse(args) // ExitOnError: Parse never returns a non-nil error here

	logger := zap.New(zap.UseFlagOptions(&zapOpts))
	ctrl.SetLogger(logger)
	log := logger.WithName("runner")

	cfg, err := ConfigFromEnv()
	if err != nil {
		log.Error(err, "invalid runner environment")
		return 1
	}
	cfg.MaxConcurrentChecks = *maxConcurrent

	scheme := runtime.NewScheme()
	utilruntime.Must(verikubev1alpha1.AddToScheme(scheme))

	restCfg, err := ctrl.GetConfig()
	if err != nil {
		log.Error(err, "failed to load kubernetes client configuration")
		return 1
	}
	c, err := client.New(restCfg, client.Options{Scheme: scheme})
	if err != nil {
		log.Error(err, "failed to create kubernetes client")
		return 1
	}

	opts := checker.Options{AllowLocalTargets: *allowLocal}
	r := &Runner{
		Client: c,
		Registry: checker.NewRegistry(
			checker.NewTCPChecker(opts),
			checker.NewHTTPChecker(opts),
			checker.NewGRPCChecker(opts),
		),
		Config: cfg,
		Log:    log,
	}

	if err := r.Run(ctrl.SetupSignalHandler()); err != nil {
		log.Error(err, "runner failed")
		return 1
	}
	log.Info("all checks executed and reported")
	return 0
}
