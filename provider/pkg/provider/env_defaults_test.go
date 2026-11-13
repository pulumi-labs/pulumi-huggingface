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
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func envTestStr(s string) *string { return &s }

func envTestDeref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// Environment-only settings must reach Configure even when the engine skips
// CheckConfig, as it does for the provider of an imported resource (#1951).
func TestConfigure_EnvDefaultsWhenCheckConfigSkipped(t *testing.T) {
	t.Setenv("HUGGINGFACE_TOKEN", "")
	t.Setenv("HUGGINGFACE_TOKEN", "env-Token")
	c := ProviderConfig{}
	require.NoError(t, c.Configure(context.Background()))
	require.Equal(t, "env-Token", envTestDeref(c.Token))
}

// A value from stack config wins over the environment, and an unset
// environment leaves the setting unset.
func TestConfigure_EnvDefaultsDoNotOverrideConfig(t *testing.T) {
	t.Setenv("HUGGINGFACE_TOKEN", "env-Token")
	c := ProviderConfig{Token: envTestStr("from-config")}
	require.NoError(t, c.Configure(context.Background()))
	require.Equal(t, "from-config", envTestDeref(c.Token))
	t.Setenv("HUGGINGFACE_TOKEN", "")
	c = ProviderConfig{}
	require.NoError(t, c.Configure(context.Background()))
	require.Nil(t, c.Token)
}
