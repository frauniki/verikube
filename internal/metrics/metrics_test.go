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

package metrics

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

const lastResultHeader = `
# HELP verikube_check_last_result Latest verdict per check: 1 if it passed on every pod in the last completed run, 0 otherwise.
# TYPE verikube_check_last_result gauge
`

func TestSetLastResults(t *testing.T) {
	tests := []struct {
		name string
		sets []map[string]bool
		want string
	}{
		{
			name: "records pass and fail verdicts",
			sets: []map[string]bool{{"a": true, "b": false}},
			want: `verikube_check_last_result{check="a",namespace="ns1",suite="s1"} 1
verikube_check_last_result{check="b",namespace="ns1",suite="s1"} 0
`,
		},
		{
			name: "drops series for removed or renamed checks",
			sets: []map[string]bool{
				{"old-name": true, "kept": true},
				{"new-name": true, "kept": false},
			},
			want: `verikube_check_last_result{check="kept",namespace="ns1",suite="s1"} 0
verikube_check_last_result{check="new-name",namespace="ns1",suite="s1"} 1
`,
		},
		{
			name: "empty results clear the suite",
			sets: []map[string]bool{{"a": true}, {}},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			CheckLastResult.Reset()
			for _, results := range tt.sets {
				SetLastResults("ns1", "s1", results)
			}
			want := ""
			if tt.want != "" {
				want = lastResultHeader + tt.want
			}
			if err := testutil.CollectAndCompare(CheckLastResult, strings.NewReader(want)); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestSetLastResultsKeepsOtherSuites(t *testing.T) {
	CheckLastResult.Reset()
	SetLastResults("ns1", "s1", map[string]bool{"a": true})
	SetLastResults("ns1", "s2", map[string]bool{"a": false})
	SetLastResults("ns2", "s1", map[string]bool{"a": false})

	SetLastResults("ns1", "s1", map[string]bool{"b": true})

	want := lastResultHeader + `verikube_check_last_result{check="a",namespace="ns1",suite="s2"} 0
verikube_check_last_result{check="a",namespace="ns2",suite="s1"} 0
verikube_check_last_result{check="b",namespace="ns1",suite="s1"} 1
`
	if err := testutil.CollectAndCompare(CheckLastResult, strings.NewReader(want)); err != nil {
		t.Error(err)
	}
}

func TestDeleteSuite(t *testing.T) {
	CheckLastResult.Reset()
	CheckRunLastCompletion.Reset()
	SetLastResults("ns1", "gone", map[string]bool{"a": true})
	SetLastResults("ns1", "stays", map[string]bool{"a": true})
	CheckRunLastCompletion.WithLabelValues("ns1", "gone").Set(1000)
	CheckRunLastCompletion.WithLabelValues("ns1", "stays").Set(2000)

	DeleteSuite("ns1", "gone")

	wantResults := lastResultHeader + `verikube_check_last_result{check="a",namespace="ns1",suite="stays"} 1
`
	if err := testutil.CollectAndCompare(CheckLastResult, strings.NewReader(wantResults)); err != nil {
		t.Error(err)
	}
	wantCompletion := `
# HELP verikube_checkrun_last_completion_timestamp_seconds Unix timestamp of the suite's last completed CheckRun with results.
# TYPE verikube_checkrun_last_completion_timestamp_seconds gauge
verikube_checkrun_last_completion_timestamp_seconds{namespace="ns1",suite="stays"} 2000
`
	if err := testutil.CollectAndCompare(CheckRunLastCompletion, strings.NewReader(wantCompletion)); err != nil {
		t.Error(err)
	}
}
