/*
 * Copyright 2018-2020 The NATS Authors
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
	"sort"
	"strconv"
	"strings"
	"time"

	cli "github.com/nats-io/cliprompts/v2"
	"github.com/nats-io/jwt"
	"github.com/nats-io/nkeys"
	"github.com/nats-io/nsc/cmd/store"
	"github.com/spf13/cobra"
)

func CreateAddUserCmd() *cobra.Command {
	var params AddUserParams
	cmd := &cobra.Command{
		Use:          "user",
		Short:        "Add an user to the account",
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		Example: `# Add user with a previously generated public key:
nsc add user --name <n> --public-key <nkey>
# Note: that unless you specify the seed, the key won't be stored in the keyring.'

# Set permissions so that the user can publish and/or subscribe to the specified subjects or wildcards:
nsc add user --name <n> --allow-pubsub <subject>,...
nsc add user --name <n> --allow-pub <subject>,...
nsc add user --name <n> --allow-sub <subject>,...

# Set permissions so that the user cannot publish nor subscribe to the specified subjects or wildcards:
nsc add user --name <n> --deny-pubsub <subject>,...
nsc add user --name <n> --deny-pub <subject>,...
nsc add user --name <n> --deny-sub <subject>,...

# To dynamically allow publishing to reply subjects, this works well for service responders:
nsc add user --name <n> --allow-pub-response

# A permission to publish a response can be removed after a duration from when 
# the message was received:
nsc add user --name <n> --allow-pub-response --response-ttl 5s

# If the service publishes multiple response messages, you can specify:
nsc add user --name <n> --allow-pub-response=5
# See 'nsc edit export --response-type --help' to enable multiple
# responses between accounts
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunAction(cmd, args, &params)
		},
	}

	cmd.Flags().StringSliceVarP(&params.allowPubs, "allow-pub", "", nil, "publish permissions - comma separated list or option can be specified multiple times")
	cmd.Flags().StringSliceVarP(&params.allowPubsub, "allow-pubsub", "", nil, "publish and subscribe permissions - comma separated list or option can be specified multiple times")
	cmd.Flags().StringSliceVarP(&params.allowSubs, "allow-sub", "", nil, "subscribe permissions - comma separated list or option can be specified multiple times")

	cmd.Flags().StringSliceVarP(&params.denyPubs, "deny-pub", "", nil, "deny publish permissions - comma separated list or option can be specified multiple times")
	cmd.Flags().StringSliceVarP(&params.denyPubsub, "deny-pubsub", "", nil, "deny publish and subscribe permissions - comma separated list or option can be specified multiple times")
	cmd.Flags().StringSliceVarP(&params.denySubs, "deny-sub", "", nil, "deny subscribe permissions - comma separated list or option can be specified multiple times")

	cmd.Flags().StringSliceVarP(&params.tags, "tag", "", nil, "tags for user - comma separated list or option can be specified multiple times")
	cmd.Flags().StringSliceVarP(&params.src, "source-network", "", nil, "source network for connection - comma separated list or option can be specified multiple times")

	cmd.Flags().StringVarP(&params.name, "name", "n", "", "name to assign the user")
	cmd.Flags().StringVarP(&params.keyPath, "public-key", "k", "", "public key identifying the user")

	params.TimeParams.BindFlags(cmd)
	params.AccountContextParams.BindFlags(cmd)
	params.ResponsePermsParams.bindSetFlags(cmd)

	return cmd
}

func init() {
	addCmd.AddCommand(CreateAddUserCmd())
}

type AddUserParams struct {
	AccountContextParams
	SignerParams
	Entity
	TimeParams
	ResponsePermsParams
	allowPubs     []string
	allowPubsub   []string
	allowSubs     []string
	denyPubs      []string
	denyPubsub    []string
	denySubs      []string
	src           []string
	tags          []string
	credsFilePath string
}

func (p *AddUserParams) longHelp() string {
	s := `toolName add user -i
toolName add user --name u --deny-pubsub "bar.>"
toolName add user --name u --tag test,service_a`

	return strings.Replace(s, "toolName", GetToolName(), -1)
}

func (p *AddUserParams) SetDefaults(ctx ActionCtx) error {
	p.name = NameFlagOrArgument(p.name, ctx)
	if p.name == "*" {
		p.name = GetRandomName(0)
	}
	if err := p.AccountContextParams.SetDefaults(ctx); err != nil {
		return err
	}
	p.SignerParams.SetDefaults(nkeys.PrefixByteAccount, true, ctx)
	p.create = true
	p.Entity.kind = nkeys.PrefixByteUser
	p.editFn = p.editUserClaim

	return nil
}

func (p *AddUserParams) PreInteractive(ctx ActionCtx) error {
	var err error
	if err = p.AccountContextParams.Edit(ctx); err != nil {
		return err
	}

	if err = p.Entity.Edit(); err != nil {
		return err
	}

	// FIXME: we won't do interactive on the response params until pub/sub/deny permissions are interactive
	//if err := p.ResponsePermsParams.Edit(false); err != nil {
	//	return err
	//}

	if err = p.TimeParams.Edit(); err != nil {
		return err
	}

	if err = p.SignerParams.Edit(ctx); err != nil {
		return err
	}

	return nil
}

func (p *AddUserParams) Load(_ ActionCtx) error {
	return nil
}

func (p *AddUserParams) PostInteractive(_ ActionCtx) error {
	return nil
}

func (p *AddUserParams) Validate(ctx ActionCtx) error {
	var err error
	if p.name == "" {
		ctx.CurrentCmd().SilenceUsage = false
		return fmt.Errorf("user name is required")
	}

	if p.name == "*" {
		p.name = GetRandomName(0)
	}

	if err = p.AccountContextParams.Validate(ctx); err != nil {
		return err
	}

	if err = p.SignerParams.Resolve(ctx); err != nil {
		return err
	}

	if err := p.TimeParams.Validate(); err != nil {
		return err
	}

	if err := p.ResponsePermsParams.Validate(); err != nil {
		return err
	}

	return p.Entity.Valid()
}

func (p *AddUserParams) Run(ctx ActionCtx) (store.Status, error) {
	var rs store.Status
	var err error

	if err := p.Entity.StoreKeys(p.AccountContextParams.Name); err != nil {
		return nil, err
	}

	r := store.NewDetailedReport(false)
	rs, err = p.Entity.GenerateClaim(p.signerKP, ctx)
	if rs != nil {
		r.Add(rs)
	}
	if err != nil {
		r.AddFromError(err)
	}
	if rs != nil {
		r.Add(rs)
	}

	pk, _ := p.kp.PublicKey()
	if p.generated {
		r.AddOK("generated and stored user key %q", pk)
	}
	// if they gave us a seed, it stored - try to get it
	ks := ctx.StoreCtx().KeyStore
	if ks.HasPrivateKey(pk) {
		d, err := GenerateConfig(ctx.StoreCtx().Store, p.AccountContextParams.Name, p.name, p.kp)
		if err != nil {
			r.AddError("unable to save creds: %v", err)
		} else {
			p.credsFilePath, err = ks.MaybeStoreUserCreds(p.AccountContextParams.Name, p.name, d)
			if err != nil {
				r.AddError("error storing creds: %v", err)
			} else {
				r.AddOK("generated user creds file %q", AbbrevHomePaths(p.credsFilePath))
			}
		}
	} else {
		r.AddOK("skipped generating creds file - user private key is not available")
	}
	if r.HasNoErrors() {
		r.AddOK("added user %q to account %q", p.name, p.AccountContextParams.Name)
	}
	return r, nil
}

func (p *AddUserParams) editUserClaim(c interface{}, ctx ActionCtx) error {
	uc, ok := c.(*jwt.UserClaims)
	if !ok {
		return errors.New("unable to cast to user claim")
	}

	if p.TimeParams.IsStartChanged() {
		uc.NotBefore, _ = p.TimeParams.StartDate()
	}

	if p.TimeParams.IsExpiryChanged() {
		uc.Expires, _ = p.TimeParams.ExpiryDate()
	}

	if _, err := p.ResponsePermsParams.Run(uc, ctx); err != nil {
		return err
	}

	uc.Permissions.Pub.Allow.Add(p.allowPubs...)
	uc.Permissions.Pub.Allow.Add(p.allowPubsub...)
	sort.Strings(uc.Pub.Allow)

	uc.Permissions.Pub.Deny.Add(p.denyPubs...)
	uc.Permissions.Pub.Deny.Add(p.denyPubsub...)
	sort.Strings(uc.Permissions.Pub.Deny)

	uc.Permissions.Sub.Allow.Add(p.allowSubs...)
	uc.Permissions.Sub.Allow.Add(p.allowPubsub...)
	sort.Strings(uc.Permissions.Sub.Allow)

	uc.Permissions.Sub.Deny.Add(p.denySubs...)
	uc.Permissions.Sub.Deny.Add(p.denyPubsub...)
	sort.Strings(uc.Permissions.Sub.Deny)

	uc.Tags.Add(p.tags...)
	sort.Strings(uc.Tags)

	return nil
}

type ResponsePermsParams struct {
	respTTL string
	respMax int
	rmResp  bool
}

func (p *ResponsePermsParams) bindSetFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&p.respTTL, "response-ttl", "", "", "the amount of time the permission is valid (global) - [#ms(millis) | #s(econds) | m(inutes) | h(ours)] - Default is no time limit.")

	cmd.Flags().IntVarP(&p.respMax, "allow-pub-response", "", 0, "client can publish only to reply subjects [with an optional count] (global)")
	cmd.Flag("allow-pub-response").NoOptDefVal = "1"

	cmd.Flags().IntVarP(&p.respMax, "max-responses", "", 0, "client can publish only to reply subjects [with an optional count] (global)")
	cmd.Flag("max-responses").Hidden = true
	cmd.Flag("max-responses").Deprecated = "use --allow-pub-n-responses or --allow-pub-response"
}

func (p *ResponsePermsParams) bindRemoveFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVarP(&p.rmResp, "rm-response-perms", "", false, "remove response settings")
}

func (p *ResponsePermsParams) maxResponseValidator(s string) error {
	_, err := p.parseMaxResponse(s)
	return err
}

func (p *ResponsePermsParams) parseMaxResponse(s string) (int, error) {
	if s == "" {
		return 0, nil
	}
	return strconv.Atoi(s)
}

func (p *ResponsePermsParams) ttlValidator(s string) error {
	_, err := p.parseTTL(s)
	return err
}

func (p *ResponsePermsParams) parseTTL(s string) (time.Duration, error) {
	if s == "" {
		return time.Duration(0), nil
	}
	return time.ParseDuration(s)
}

func (p *ResponsePermsParams) Edit(hasPerm bool) error {
	verb := "Set"
	if hasPerm {
		verb = "Edit"
	}
	ok, err := cli.Confirm(fmt.Sprintf("%s response permissions?", verb), false)
	if err != nil {
		return err
	}
	if ok {
		if hasPerm {
			p.rmResp, err = cli.Confirm("delete response permission", p.rmResp)
			if err != nil {
				return err
			}
		}
		if !p.rmResp {
			s, err := cli.Prompt("Max number of responses", fmt.Sprintf("%d", p.respMax), cli.Val(p.maxResponseValidator))
			if err != nil {
				return err
			}
			p.respMax, _ = p.parseMaxResponse(s)
			p.respTTL, err = cli.Prompt("Response TTL", p.respTTL, cli.Val(p.ttlValidator))
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *ResponsePermsParams) Validate() error {
	if err := p.ttlValidator(p.respTTL); err != nil {
		return err
	}
	return nil
}

func (p *ResponsePermsParams) Run(uc *jwt.UserClaims, ctx ActionCtx) (*store.Report, error) {
	r := store.NewDetailedReport(true)
	if p.rmResp {
		uc.Resp = nil
		r.AddOK("removed response permissions")
		return r, nil
	}
	if ctx.CurrentCmd().Flag("max-responses").Changed || p.respMax != 0 {
		if uc.Resp == nil {
			uc.Resp = &jwt.ResponsePermission{}
		}
		uc.Resp.MaxMsgs = p.respMax
		r.AddOK("set max responses to %d", p.respMax)
	}

	if p.respTTL != "" {
		v, err := p.parseTTL(p.respTTL)
		if err != nil {
			return nil, err
		}
		if uc.Resp == nil {
			uc.Resp = &jwt.ResponsePermission{}
		}
		uc.Resp.Expires = v
		r.AddOK("set response ttl to %v", v)
	}
	return r, nil
}
