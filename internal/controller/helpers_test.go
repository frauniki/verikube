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
	verikubev1alpha1 "github.com/frauniki/verikube/api/v1alpha1"
)

// validTemplate returns a minimal CheckSuiteTemplate that passes CRD validation.
func validTemplate() verikubev1alpha1.CheckSuiteTemplate {
	return verikubev1alpha1.CheckSuiteTemplate{
		Runners: []verikubev1alpha1.RunnerSpec{
			{Name: defaultRunnerName},
		},
		Checks: []verikubev1alpha1.CheckSpec{
			{
				Name: "example-tcp",
				TCP:  &verikubev1alpha1.TCPCheck{Address: "example.com:443"},
			},
		},
	}
}
