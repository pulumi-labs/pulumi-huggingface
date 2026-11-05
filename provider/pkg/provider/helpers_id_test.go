// Copyright 2026, Pulumi Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package provider

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// #1549: the Hub's create response carries an ObjectId in id (the fake now
// does too), so the resource ID must come from namespace/name. State written
// by v0.1.0 holds the ObjectId and gets an actionable error.
func TestSplitID_LegacyObjectIDIsActionable(t *testing.T) {
	_, _, err := splitID("65f0c0ffee0000000000abcd")
	require.ErrorContains(t, err, "pulumi state delete")
	require.ErrorContains(t, err, "pulumi import")
	_, _, err = splitID("/x")
	require.NotContains(t, err.Error(), "pulumi state delete")
}
