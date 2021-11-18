package cloudx

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"syscall"

	cloud "github.com/ory/client-go"
	"github.com/ory/x/pointerx"

	"github.com/gofrs/uuid/v3"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/tidwall/gjson"
	"golang.org/x/term"

	kratos "github.com/ory/kratos-client-go"
	"github.com/ory/x/cmdx"
	"github.com/ory/x/flagx"
	"github.com/ory/x/stringsx"
)

const (
	fileName   = ".ory-cloud.json"
	configFlag = "cloud-config"
	quietFlag  = "quiet"
	osEnvVar   = "ORY_CLOUD_CONFIG_PATH"
	cloudUrl   = "ORY_CLOUD_URL"
	version    = "v0alpha0"
)

func RegisterFlags(f *pflag.FlagSet) {
	f.String(configFlag, "", "Path to the Ory Cloud configuration file.")
	f.Bool(quietFlag, false, "Do not print any output.")
}

type AuthContext struct {
	Version         string       `json:"version"`
	SessionToken    string       `json:"session_token"`
	SelectedProject uuid.UUID    `json:"selected_project"`
	IdentityTraits  AuthIdentity `json:"session_identity_traits"`
}

type AuthIdentity struct {
	ID    uuid.UUID
	Email string `json:"email"`
}

type AuthProject struct {
	ID   uuid.UUID `json:"id"`
	Slug string    `json:"slug"`
}

var ErrNoConfig = errors.New("no ory configuration file present")

func getConfigPath(cmd *cobra.Command) (string, error) {
	path, err := os.UserHomeDir()
	if err != nil {
		return "", errors.Wrapf(err, "unable to guess your home directory")
	}

	return stringsx.Coalesce(
		os.Getenv(osEnvVar),
		flagx.MustGetString(cmd, configFlag),
		filepath.Join(path, fileName),
	), nil
}

type SnakeCharmer struct {
	ctx              context.Context
	verboseWriter    io.Writer
	verboseErrWriter io.Writer
	configLocation   string
	noConfirm        bool
	apiDomain        *url.URL
	stdin            *bufio.Reader
	pwReader         passwordReader
}

const PasswordReader = "password_reader"

// NewSnakeCharmer creates a new SnakeCharmer instance which handles cobra CLI commands.
func NewSnakeCharmer(cmd *cobra.Command) (*SnakeCharmer, error) {
	location, err := getConfigPath(cmd)
	if err != nil {
		return nil, err
	}

	var out = cmd.OutOrStdout()
	if flagx.MustGetBool(cmd, quietFlag) {
		out = io.Discard
	}

	var outErr = cmd.OutOrStderr()
	if flagx.MustGetBool(cmd, quietFlag) {
		outErr = io.Discard
	}

	toParse := stringsx.Coalesce(
		os.Getenv(cloudUrl),
		"https://project.console.ory.sh",
	)

	apiDomain, err := url.Parse(toParse)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid API endpoint provided: %s", toParse)
	}

	pwReader := func() ([]byte, error) {
		return term.ReadPassword(syscall.Stdin)
	}
	if p, ok := cmd.Context().Value(PasswordReader).(passwordReader); ok {
		pwReader = p
	}

	return &SnakeCharmer{
		configLocation:   location,
		noConfirm:        flagx.MustGetBool(cmd, quietFlag),
		verboseWriter:    out,
		verboseErrWriter: outErr,
		stdin:            bufio.NewReader(cmd.InOrStdin()),
		apiDomain:        apiDomain,
		ctx:              cmd.Context(),
		pwReader:         pwReader,
	}, nil
}

func (h *SnakeCharmer) Stdin() *bufio.Reader {
	return h.stdin
}

func (h *SnakeCharmer) WriteConfig(c *AuthContext) error {
	c.Version = version
	file, err := os.OpenFile(h.configLocation, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return errors.Wrapf(err, "unable to open file for writing at location: %s", file.Name())
	}
	defer file.Close()

	if err := json.NewEncoder(file).Encode(c); err != nil {
		return errors.Wrapf(err, "unable to write configuration to file: %s", h.configLocation)
	}

	return nil
}

func (h *SnakeCharmer) readConfig() (*AuthContext, error) {
	file, err := os.Open(h.configLocation)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return new(AuthContext), ErrNoConfig
		}
		return nil, errors.Wrapf(err, "unable to open ory config file location: %s", h.configLocation)
	}
	defer file.Close()

	var c AuthContext
	if err := json.NewDecoder(file).Decode(&c); err != nil {
		return nil, errors.Wrapf(err, "unable to JSON decode the ory config file: %s", h.configLocation)
	}

	return &c, nil
}

func (h *SnakeCharmer) EnsureContext() (*AuthContext, error) {
	c, err := h.readConfig()
	if err != nil {
		if errors.Is(err, ErrNoConfig) && !h.noConfirm {
			// Continue to sign in
		} else {
			return nil, err
		}
	}

	if len(c.SessionToken) > 0 {
		if h.noConfirm {
			ok, err := cmdx.AskScannerForConfirmation("Press [y] to continue as that user or [n] to sign into another account.", h.stdin, h.verboseWriter)
			if err != nil {
				return nil, err
			}
			if ok {
				if err := h.SignOut(); err != nil {
					return nil, err
				}
				c = new(AuthContext)
				c, err = h.Authenticate()
				if err != nil {
					return nil, err
				}
			}
		}

		return c, nil
	} else {
		c, err = h.Authenticate()
		if err != nil {
			return nil, err
		}
	}

	if len(c.SessionToken) == 0 {
		return nil, errors.Errorf("unable to authenticate")
	}

	return c, nil
}

func (h *SnakeCharmer) getField(i interface{}, path string) (*gjson.Result, error) {
	var b bytes.Buffer
	if err := json.NewEncoder(&b).Encode(i); err != nil {
		return nil, err
	}
	result := gjson.GetBytes(b.Bytes(), path)
	return &result, nil
}

func (h *SnakeCharmer) signup(c *kratos.APIClient) (*AuthContext, error) {
	flow, _, err := c.V0alpha2Api.InitializeSelfServiceRegistrationFlowWithoutBrowser(h.ctx).Execute()
	if err != nil {
		return nil, err
	}

	var isRetry bool
retryRegistration:
	if isRetry {
		_, _ = fmt.Fprintf(h.verboseErrWriter, "\nYour account creation attempt failed. Please try again!\n\n")
	}
	isRetry = true

	var form kratos.SubmitSelfServiceRegistrationFlowWithPasswordMethodBody
	if err := renderForm(h.stdin, h.pwReader, h.verboseWriter, flow.Ui, "password", &form); err != nil {
		return nil, err
	}

	signup, _, err := c.V0alpha2Api.SubmitSelfServiceRegistrationFlow(h.ctx).
		Flow(flow.Id).SubmitSelfServiceRegistrationFlowBody(kratos.SubmitSelfServiceRegistrationFlowBody{
		SubmitSelfServiceRegistrationFlowWithPasswordMethodBody: &form,
	}).Execute()
	if err != nil {
		if e, ok := err.(*kratos.GenericOpenAPIError); ok {
			switch m := e.Model().(type) {
			case *kratos.SelfServiceRegistrationFlow:
				flow = m
				goto retryRegistration
			case kratos.SelfServiceRegistrationFlow:
				flow = &m
				goto retryRegistration
			}
		}

		return nil, errors.WithStack(err)
	}

	sessionToken := *signup.SessionToken
	sess, _, err := c.V0alpha2Api.ToSession(h.ctx).XSessionToken(sessionToken).Execute()
	if err != nil {
		return nil, err
	}

	return h.sessionToContext(sess, sessionToken)
}

func (h *SnakeCharmer) signin(c *kratos.APIClient, sessionToken string) (*AuthContext, error) {
	req := c.V0alpha2Api.InitializeSelfServiceLoginFlowWithoutBrowser(h.ctx)
	if len(sessionToken) > 0 {
		req = req.XSessionToken(sessionToken).Aal("aal2")
	}

	flow, _, err := req.Execute()
	if err != nil {
		return nil, err
	}

	var isRetry bool
retryLogin:
	if isRetry {
		_, _ = fmt.Fprintf(h.verboseErrWriter, "\nYour sign in attempt failed. Please try again!\n\n")
	}
	isRetry = true

	var form interface{} = &kratos.SubmitSelfServiceLoginFlowWithPasswordMethodBody{}
	method := "password"
	if len(sessionToken) > 0 {
		var foundTOTP bool
		var foundLookup bool
		for _, n := range flow.Ui.Nodes {
			if n.Group == "totp" {
				foundTOTP = true
			} else if n.Group == "lookup_secret" {
				foundLookup = true
			}
		}
		if !foundLookup && !foundTOTP {
			return nil, errors.New("only TOTP and lookup secrets are supported for two-step verification in the CLI")
		}

		method = "lookup_secret"
		if foundTOTP {
			form = &kratos.SubmitSelfServiceLoginFlowWithTotpMethodBody{}
			method = "totp"
		}
	}

	if err := renderForm(h.stdin, h.pwReader, h.verboseWriter, flow.Ui, method, form); err != nil {
		return nil, err
	}

	var body kratos.SubmitSelfServiceLoginFlowBody
	switch e := form.(type) {
	case *kratos.SubmitSelfServiceLoginFlowWithTotpMethodBody:
		body.SubmitSelfServiceLoginFlowWithTotpMethodBody = e
	case *kratos.SubmitSelfServiceLoginFlowWithPasswordMethodBody:
		body.SubmitSelfServiceLoginFlowWithPasswordMethodBody = e
	default:
		panic("unexpected type")
	}

	login, _, err := c.V0alpha2Api.SubmitSelfServiceLoginFlow(h.ctx).XSessionToken(sessionToken).
		Flow(flow.Id).SubmitSelfServiceLoginFlowBody(body).Execute()
	if err != nil {
		if e, ok := err.(*kratos.GenericOpenAPIError); ok {
			switch m := e.Model().(type) {
			case *kratos.SelfServiceLoginFlow:
				flow = m
				goto retryLogin
			case kratos.SelfServiceLoginFlow:
				flow = &m
				goto retryLogin
			}
		}

		return nil, errors.WithStack(err)
	}

	sessionToken = stringsx.Coalesce(*login.SessionToken, sessionToken)
	sess, _, err := c.V0alpha2Api.ToSession(h.ctx).XSessionToken(sessionToken).Execute()
	if err == nil {
		return h.sessionToContext(sess, sessionToken)
	}

	if e, ok := err.(*kratos.GenericOpenAPIError); ok {
		switch gjson.GetBytes(e.Body(), "error.id").String() {
		case "session_aal2_required":
			return h.signin(c, sessionToken)
		}
	}
	return nil, err
}

func (h *SnakeCharmer) sessionToContext(session *kratos.Session, token string) (*AuthContext, error) {
	email, err := h.getField(session.Identity.Traits, "email")
	if err != nil {
		return nil, err
	}

	return &AuthContext{
		Version:      version,
		SessionToken: token,
		IdentityTraits: AuthIdentity{
			Email: email.String(),
			ID:    uuid.FromStringOrNil(session.Identity.Id),
		},
	}, nil
}

func (h *SnakeCharmer) Authenticate() (*AuthContext, error) {
	if h.noConfirm {
		return nil, errors.New("can not sign in or sign up when flag --quiet is set.")
	}

	ac, err := h.readConfig()
	if err != nil {
		if !errors.Is(err, ErrNoConfig) {
			return nil, err
		}
	}

	if len(ac.SessionToken) > 0 {
		ok, err := cmdx.AskScannerForConfirmation(fmt.Sprintf("You are signed in as \"%s\" already. Do you wish to authenticate with another account?", ac.IdentityTraits.Email), h.stdin, h.verboseWriter)
		if err != nil {
			return nil, err
		}
		if !ok {
			return ac, nil
		}

		_, _ = fmt.Fprintf(h.verboseErrWriter, "Ok, signing you out!\n")
		if err := h.SignOut(); err != nil {
			return nil, err
		}
	}

	c, err := newKratosClient("public")
	if err != nil {
		return nil, err
	}

	signIn, err := cmdx.AskScannerForConfirmation("Do you already have an Ory Console account you wish to use?", h.stdin, h.verboseWriter)
	if err != nil {
		return nil, err
	}

	var retry bool
	if retry {
		_, _ = fmt.Fprintln(h.verboseErrWriter, "Unable to Authenticate you, please try again.")
	}

	if signIn {
		ac, err = h.signin(c, "")
		if err != nil {
			return nil, err
		}
	} else {
		_, _ = fmt.Fprintln(h.verboseErrWriter, "Great to have you here, creating an Ory Cloud account is absolutely free and only requires to answer four easy questions.")

		ac, err = h.signup(c)
		if err != nil {
			return nil, err
		}
	}

	if err := h.WriteConfig(ac); err != nil {
		return nil, err
	}

	_, _ = fmt.Fprintf(h.verboseErrWriter, "You are now signed in as: %s\n", ac.IdentityTraits.Email)

	return ac, nil
}

func (h *SnakeCharmer) SignOut() error {
	return h.WriteConfig(new(AuthContext))
}

func (h *SnakeCharmer) ListProjects() ([]cloud.Project, error) {
	ac, err := h.EnsureContext()
	if err != nil {
		return nil, err
	}

	c, err := newCloudClient(ac.SessionToken)
	if err != nil {
		return nil, err
	}

	projects, res, err := c.V0alpha0Api.ListProjects(h.ctx).Execute()
	if err != nil {
		return nil, handleError("unable to list projects", res, err)
	}

	return projects, nil
}

func (h *SnakeCharmer) CreateProject(name, preset string) (*cloud.Project, error) {
	ac, err := h.EnsureContext()
	if err != nil {
		return nil, err
	}

	c, err := newCloudClient(ac.SessionToken)
	if err != nil {
		return nil, err
	}

	project, res, err := c.V0alpha0Api.CreateProject(h.ctx).ProjectPatch(cloud.ProjectPatch{
		DefaultIdentitySchemaUrl: pointerx.String(preset),
		Name:                     pointerx.String(name),
	}).Execute()
	if err != nil {
		return nil, handleError("unable to list projects", res, err)
	}

	return project, nil
}

func handleError(message string, res *http.Response, err error) error {
	if e, ok := err.(*kratos.GenericOpenAPIError); ok {
		return errors.Wrapf(err, "%s: %s", message, e.Body())
	}
	body, _ := ioutil.ReadAll(res.Body)
	return errors.Wrapf(err, "%s: %s", message, body)
}

//func (h *SnakeCharmer) UpdateProject() (interface{}, error) {
//	ac, err := h.EnsureContext()
//	if err != nil {
//		return nil, err
//	}
//
//	hc := NewConsoleHTTPClient(ac.SessionToken)
//	hc.Get()
//
//	return nil, nil
//}
