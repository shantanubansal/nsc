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
	"errors"
	"fmt"

	"github.com/nats-io/jwt"
	"github.com/nats-io/nkeys"
	"github.com/nats-io/nsc/cmd/store"
	"github.com/spf13/cobra"
)

func createAddAccountCmd() *cobra.Command {
	var params AddAccountParams
	cmd := &cobra.Command{
		Use:   "account",
		Short: "Add an account",
		RunE: func(cmd *cobra.Command, args []string) error {

			if err := params.Validate(); err != nil {
				return err
			}

			if err := params.Run(); err != nil {
				return err
			}

			if params.generate {
				cmd.Printf("Generated account key - private key stored %q\n", params.accountKeyPath)
			} else {
				cmd.Printf("Success! - added account %q\n", params.Name)
			}

			return nil
		},
	}
	cmd.Flags().StringVarP(&params.Name, "name", "", "", "account name")

	cmd.Flags().StringVarP(&params.accountKeyPath, "public-key", "k", "", "public key identifying the account")
	cmd.Flags().BoolVarP(&params.generate, "generate-nkeys", "", false, "generate nkeys")

	cmd.MarkFlagRequired("name")

	return cmd
}

func init() {
	addCmd.AddCommand(createAddAccountCmd())
}

type AddAccountParams struct {
	operatorKP     nkeys.KeyPair
	accountKP      nkeys.KeyPair
	accountKeyPath string
	generate       bool
	jwt.AccountClaims
}

func (p *AddAccountParams) Validate() error {
	s, err := getStore()
	if err != nil {
		return err
	}

	if p.accountKeyPath != "" && p.generate {
		return errors.New("specify one of --public-key or --generate-nkeys")
	}

	if s.Has(store.Accounts, p.Name) {
		return fmt.Errorf("account %q already exists", p.Name)
	}

	ctx, err := s.GetContext()
	if err != nil {
		return fmt.Errorf("error getting context: %v", err)
	}

	p.operatorKP, err = ctx.ResolveKey(nkeys.PrefixByteOperator, store.KeyPathFlag)
	if err != nil {
		return fmt.Errorf("specify the operator private key with --private-key to use for signing the cluster")
	}

	if p.generate {
		p.accountKP, err = nkeys.CreateAccount()
		if err != nil {
			return fmt.Errorf("error generating an account key: %v", err)
		}
	} else {
		p.accountKP, err = ctx.ResolveKey(nkeys.PrefixByteAccount, p.accountKeyPath)
		if err != nil {
			return fmt.Errorf("error resolving account key: %v", err)
		}
	}

	return nil
}

func (p *AddAccountParams) Run() error {
	pkd, err := p.accountKP.PublicKey()
	if err != nil {
		return err
	}
	p.Subject = string(pkd)
	token, err := p.AccountClaims.Encode(p.operatorKP)
	if err != nil {
		return err
	}

	s, err := getStore()
	if err != nil {
		return err
	}

	if err := s.StoreClaim([]byte(token)); err != nil {
		return err
	}

	if p.generate {
		ks := store.NewKeyStore()
		p.accountKeyPath, err = ks.Store(s.Info.Name, p.Name, p.accountKP)
		if err != nil {
			return err
		}
	}

	return nil
}