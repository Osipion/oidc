package defaults

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/caos/oidc/pkg/oidc/grants"

	"golang.org/x/oauth2"

	"github.com/caos/oidc/pkg/oidc"
	grants_tx "github.com/caos/oidc/pkg/oidc/grants/tokenexchange"
	"github.com/caos/oidc/pkg/rp"
	"github.com/caos/oidc/pkg/rp/tokenexchange"
	"github.com/caos/oidc/pkg/utils"
)

const (
	idTokenKey = "id_token"
	stateParam = "state"
)

type DefaultRP struct {
	endpoints rp.Endpoints

	oauthConfig oauth2.Config
	config      *rp.Config

	httpClient    *http.Client
	cookieHandler *utils.CookieHandler

	verifier rp.Verifier
}

func NewDefaultRelayingParty(rpConfig *rp.Config, rpOpts ...DefaultReplayingPartyOpts) (tokenexchange.DelegationTokenExchangeRP, error) {
	p := &DefaultRP{
		config:     rpConfig,
		httpClient: utils.DefaultHTTPClient,
	}

	for _, optFunc := range rpOpts {
		optFunc(p)
	}

	if err := p.discover(); err != nil {
		return nil, err
	}

	if p.verifier == nil {
		// p.verifier = NewVerifier(rpConfig.Issuer, rpConfig.ClientID, utils.NewRemoteKeySet(p.httpClient, p.endpoints.JKWsURL)) //TODO: keys endpoint
	}

	return p, nil
}

type DefaultReplayingPartyOpts func(p *DefaultRP)

func WithCookieHandler(cookieHandler *utils.CookieHandler) DefaultReplayingPartyOpts {
	return func(p *DefaultRP) {
		p.cookieHandler = cookieHandler
	}
}

func WithHTTPClient(client *http.Client) DefaultReplayingPartyOpts {
	return func(p *DefaultRP) {
		p.httpClient = client
	}
}

func (p *DefaultRP) AuthURL(state string) string {
	return p.oauthConfig.AuthCodeURL(state)
}

func (p *DefaultRP) AuthURLHandler(state string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := p.trySetStateCookie(w, state); err != nil {
			http.Error(w, "failed to create state cookie: "+err.Error(), http.StatusUnauthorized)
			return
		}
		http.Redirect(w, r, p.AuthURL(state), http.StatusFound)
	}
}

func (p *DefaultRP) CodeExchange(ctx context.Context, code string) (tokens *oidc.Tokens, err error) {
	token, err := p.oauthConfig.Exchange(ctx, code)
	if err != nil {
		return nil, err //TODO: our error
	}
	idTokenString, ok := token.Extra(idTokenKey).(string)
	if !ok {
		//TODO: implement
	}

	idToken, err := p.verifier.Verify(ctx, token.AccessToken, idTokenString)
	if err != nil {
		return nil, err //TODO: err
	}

	return &oidc.Tokens{Token: token, IDTokenClaims: idToken}, nil
}

func (p *DefaultRP) CodeExchangeHandler(callback func(http.ResponseWriter, *http.Request, *oidc.Tokens, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state, err := p.tryReadStateCookie(w, r)
		if err != nil {
			http.Error(w, "failed to get state: "+err.Error(), http.StatusUnauthorized)
			return
		}
		tokens, err := p.CodeExchange(r.Context(), r.URL.Query().Get("code"))
		if err != nil {
			http.Error(w, "failed to exchange token: "+err.Error(), http.StatusUnauthorized)
			return
		}
		callback(w, r, tokens, state)
	}
}

// func (p *DefaultRP) Introspect(ctx context.Context, accessToken string) (oidc.TokenIntrospectResponse, error) {
// 	// req := &http.Request{}
// 	// resp, err := p.httpClient.Do(req)
// 	// if err != nil {

// 	// }
// 	// p.endpoints.IntrospectURL
// 	return nil, nil
// }

func (p *DefaultRP) Userinfo() {}

func (p *DefaultRP) TokenExchange(ctx context.Context, request *grants_tx.TokenExchangeRequest) (newToken *oauth2.Token, err error) {
	return p.callTokenEndpoint(request)
}

func (p *DefaultRP) callTokenEndpoint(request interface{}) (newToken *oauth2.Token, err error) {
	req, err := utils.FormRequest(p.endpoints.TokenURL, request)
	if err != nil {
		return nil, err
	}
	auth := base64.StdEncoding.EncodeToString([]byte(p.config.ClientID + ":" + p.config.ClientSecret))
	req.Header.Set("Authorization", "Basic "+auth)
	token := new(oauth2.Token)
	if err := utils.HttpRequest(p.httpClient, req, token); err != nil {
		return nil, err
	}
	return token, nil
}

func (p *DefaultRP) ClientCredentials(ctx context.Context, scopes ...string) (newToken *oauth2.Token, err error) {
	return p.callTokenEndpoint(grants.ClientCredentialsGrantBasic(scopes...))
}

func (p *DefaultRP) DelegationTokenExchange(ctx context.Context, subjectToken string, reqOpts ...grants_tx.TokenExchangeOption) (newToken *oauth2.Token, err error) {
	return p.TokenExchange(ctx, DelegationTokenRequest(subjectToken, reqOpts...))
}

func (p *DefaultRP) discover() error {
	wellKnown := strings.TrimSuffix(p.config.Issuer, "/") + oidc.DiscoveryEndpoint

	req, err := http.NewRequest("GET", wellKnown, nil)
	if err != nil {
		return err
	}
	discoveryConfig := new(oidc.DiscoveryConfiguration)

	err = utils.HttpRequest(p.httpClient, req, &discoveryConfig)
	if err != nil {
		return err
	}

	p.endpoints = rp.GetEndpoints(discoveryConfig)
	p.oauthConfig = oauth2.Config{
		ClientID:     p.config.ClientID,
		ClientSecret: p.config.ClientSecret,
		Endpoint:     p.endpoints.Endpoint,
		RedirectURL:  p.config.CallbackURL,
		Scopes:       p.config.Scopes,
	}
	return nil
}

func (p *DefaultRP) trySetStateCookie(w http.ResponseWriter, state string) error {
	if p.cookieHandler != nil {
		if err := p.cookieHandler.SetQueryCookie(w, stateParam, state); err != nil {
			return err
		}
	}
	return nil
}

func (p *DefaultRP) tryReadStateCookie(w http.ResponseWriter, r *http.Request) (state string, err error) {
	if p.cookieHandler == nil {
		return r.FormValue(stateParam), nil
	}
	state, err = p.cookieHandler.CheckQueryCookie(r, stateParam)
	if err != nil {
		return "", err
	}
	p.cookieHandler.DeleteCookie(w, stateParam)
	return state, nil
}
