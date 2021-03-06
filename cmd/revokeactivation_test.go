/*
 * Copyright 2018 The NATS Authors
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package cmd

import (
	"testing"
	"time"

	"github.com/nats-io/jwt"
	"github.com/stretchr/testify/require"
)

func TestRevokeActivation(t *testing.T) {
	ts := NewTestStore(t, "revoke_clear_user")
	defer ts.Done(t)

	ts.AddAccount(t, "A")
	ts.AddExport(t, "A", jwt.Stream, "foo.>", false)
	ts.AddExport(t, "A", jwt.Service, "bar", false)
	ts.AddExport(t, "A", jwt.Service, "public", true) // we support revoking public exports

	_, pub, _ := CreateAccountKey(t)

	_, _, err := ExecuteCmd(createRevokeActivationCmd(), "--subject", "foo.bar", "--target-account", pub)
	require.NoError(t, err)

	_, _, err = ExecuteCmd(createRevokeActivationCmd(), "--subject", "bar", "--target-account", pub, "--service")
	require.NoError(t, err)

	_, _, err = ExecuteCmd(createRevokeActivationCmd(), "--subject", "public", "--target-account", pub, "--service")
	require.NoError(t, err)

	ac, err := ts.Store.ReadAccountClaim("A")
	require.NoError(t, err)
	require.Len(t, ac.Exports, 3)

	for _, exp := range ac.Exports {
		require.True(t, exp.IsRevokedAt(pub, time.Unix(0, 0)))
	}
}

func TestRevokeActivationAt(t *testing.T) {
	ts := NewTestStore(t, "test")
	defer ts.Done(t)

	ts.AddAccount(t, "A")
	ts.AddExport(t, "A", jwt.Stream, "foo.>", false)
	ts.AddExport(t, "A", jwt.Service, "bar", false)

	_, pub, _ := CreateAccountKey(t)

	_, _, err := ExecuteCmd(createRevokeActivationCmd(), "--subject", "foo.bar", "--target-account", pub, "--at", "1000")
	require.NoError(t, err)

	_, _, err = ExecuteCmd(createRevokeActivationCmd(), "--subject", "bar", "--target-account", pub, "--service", "--at", "1000")
	require.NoError(t, err)

	ac, err := ts.Store.ReadAccountClaim("A")
	require.NoError(t, err)
	require.Len(t, ac.Exports, 2)

	for _, exp := range ac.Exports {
		require.True(t, exp.IsRevokedAt(pub, time.Unix(999, 0)))
		require.False(t, exp.IsRevokedAt(pub, time.Unix(1001, 0)))
	}
}

func TestRevokeActivationForStreamInteractive(t *testing.T) {
	ts := NewTestStore(t, "test")
	defer ts.Done(t)

	ts.AddAccount(t, "A")
	ts.AddExport(t, "A", jwt.Stream, "foo.>", false)
	ts.AddExport(t, "A", jwt.Service, "bar", false)
	ts.AddAccount(t, "B")
	ts.AddExport(t, "B", jwt.Stream, "foo.>", false)
	ts.AddExport(t, "B", jwt.Service, "bar", false)

	_, pub, _ := CreateAccountKey(t)

	input := []interface{}{1, false, 0, pub, "1000"} // second account "B"
	cmd := createRevokeActivationCmd()
	HoistRootFlags(cmd)
	_, _, err := ExecuteInteractiveCmd(cmd, input, "-i")
	require.NoError(t, err)

	ac, err := ts.Store.ReadAccountClaim("A")
	require.NoError(t, err)

	for _, exp := range ac.Exports {
		require.Len(t, exp.Revocations, 0)
	}

	ac, err = ts.Store.ReadAccountClaim("B")
	require.NoError(t, err)

	for _, exp := range ac.Exports {
		if exp.Subject != "foo.>" {
			require.Len(t, exp.Revocations, 0)
			continue
		}
		require.Len(t, exp.Revocations, 1)
		require.True(t, exp.IsRevokedAt(pub, time.Unix(999, 0)))
		require.False(t, exp.IsRevokedAt(pub, time.Unix(1001, 0)))
	}
}

func TestRevokeActivationForServiceInteractive(t *testing.T) {
	ts := NewTestStore(t, "test")
	defer ts.Done(t)

	ts.AddAccount(t, "A")
	ts.AddExport(t, "A", jwt.Stream, "foo.>", false)
	ts.AddExport(t, "A", jwt.Service, "bar", false)
	ts.AddAccount(t, "B")
	ts.AddExport(t, "B", jwt.Stream, "foo.>", false)
	ts.AddExport(t, "B", jwt.Service, "bar", false)

	_, pub, _ := CreateAccountKey(t)

	input := []interface{}{1, true, 0, pub, "1000"} // second account "B"
	cmd := createRevokeActivationCmd()
	HoistRootFlags(cmd)
	_, _, err := ExecuteInteractiveCmd(cmd, input, "-i")
	require.NoError(t, err)

	ac, err := ts.Store.ReadAccountClaim("A")
	require.NoError(t, err)

	for _, exp := range ac.Exports {
		require.Len(t, exp.Revocations, 0)
	}

	ac, err = ts.Store.ReadAccountClaim("B")
	require.NoError(t, err)

	for _, exp := range ac.Exports {
		if exp.Subject != "bar" {
			require.Len(t, exp.Revocations, 0)
			continue
		}
		require.Len(t, exp.Revocations, 1)
		require.True(t, exp.IsRevokedAt(pub, time.Unix(999, 0)))
		require.False(t, exp.IsRevokedAt(pub, time.Unix(1001, 0)))
	}
}
