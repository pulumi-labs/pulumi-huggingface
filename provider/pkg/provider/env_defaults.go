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
	"os"
)

// Configure fills every provider setting that is still unset from its
// environment variable. infer applies the SetDefault environment defaults only
// in CheckConfig, but after `pulumi import` the engine reconfigures the
// provider from the persisted inputs without it, so destroy, refresh and
// update of an imported resource would otherwise ignore credentials given
// only through the environment (same bug as #1951; upstream
// https://github.com/pulumi/pulumi-go-provider/issues/527). Settings already
// present, in stack config or from the defaults, win. The provider registers
// its config as a pointer (infer.Config(&T{})) so infer sees this method.
func (c *ProviderConfig) Configure(_ context.Context) error {
	envDefault(&c.Token, "HUGGINGFACE_TOKEN")
	return nil
}

// envDefault sets *field to the first non-empty environment variable among
// names when *field is unset or empty.
func envDefault(field **string, names ...string) {
	if *field != nil && **field != "" {
		return
	}
	for _, name := range names {
		if v := os.Getenv(name); v != "" {
			*field = &v
			return
		}
	}
}
