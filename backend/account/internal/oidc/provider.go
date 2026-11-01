package oidc

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ory/fosite"
	"github.com/ory/fosite/handler/oauth2"
	"github.com/ory/fosite/handler/openid"
	"github.com/ory/fosite/handler/pkce"
	"github.com/ory/fosite/token/hmac"
	"github.com/ory/fosite/token/jwt"
)

// Lifespans for issued tokens.
const (
	AuthCodeLifespan     = 5 * time.Minute
	AccessTokenLifespan  = 15 * time.Minute
	RefreshTokenLifespan = 30 * 24 * time.Hour
	IDTokenLifespan      = time.Hour
)

// NewProvider assembles fosite with the handlers this provider supports:
// authorization code + PKCE (S256, mandatory) + OIDC ID tokens + refresh
// rotation + revocation. Adapted from the verified Zeal/Auth wiring.
func NewProvider(pool *pgxpool.Pool, keys *KeyService, issuer string, globalSecret []byte) (fosite.OAuth2Provider, error) {
	store := NewStorage(pool)

	config := &fosite.Config{
		AccessTokenLifespan:            AccessTokenLifespan,
		RefreshTokenLifespan:           RefreshTokenLifespan,
		AuthorizeCodeLifespan:          AuthCodeLifespan,
		IDTokenLifespan:                IDTokenLifespan,
		IDTokenIssuer:                  issuer,
		EnforcePKCE:                    true,
		EnforcePKCEForPublicClients:    true,
		EnablePKCEPlainChallengeMethod: false, // S256 only
		MinParameterEntropy:            8,
		AllowedPromptValues:            []string{"select_account"},
		ScopeStrategy:                  fosite.WildcardScopeStrategy,
		AudienceMatchingStrategy:       fosite.DefaultAudienceMatchingStrategy,
		RefreshTokenScopes:             []string{}, // always issue refresh tokens
		TokenURL:                       issuer + "/token",
		GlobalSecret:                   globalSecret, // HMAC key for opaque tokens
	}

	hmacStrat := oauth2.NewHMACSHAStrategy(&hmac.HMACStrategy{Config: config}, config)

	jwtSigner := &jwt.DefaultSigner{
		GetPrivateKey: func(ctx context.Context) (any, error) {
			return keys.ActivePrivateKey(ctx)
		},
	}
	oidcStrategy := &openid.DefaultStrategy{Signer: jwtSigner, Config: config}

	config.AuthorizeEndpointHandlers = fosite.AuthorizeEndpointHandlers{}
	config.TokenEndpointHandlers = fosite.TokenEndpointHandlers{}
	config.RevocationHandlers = fosite.RevocationHandlers{}
	config.TokenIntrospectionHandlers = fosite.TokenIntrospectionHandlers{}

	// Authorization code grant.
	authCodeHandler := &oauth2.AuthorizeExplicitGrantHandler{
		AccessTokenStrategy:    hmacStrat,
		RefreshTokenStrategy:   hmacStrat,
		AuthorizeCodeStrategy:  hmacStrat,
		CoreStorage:            store,
		TokenRevocationStorage: store,
		Config:                 config,
	}
	config.AuthorizeEndpointHandlers.Append(authCodeHandler)
	config.TokenEndpointHandlers.Append(authCodeHandler)

	// Refresh grant (rotation handled by the strategies + storage).
	config.TokenEndpointHandlers.Append(&oauth2.RefreshTokenGrantHandler{
		AccessTokenStrategy:    hmacStrat,
		RefreshTokenStrategy:   hmacStrat,
		TokenRevocationStorage: store,
		Config:                 config,
	})

	// PKCE.
	pkceHandler := &pkce.Handler{
		AuthorizeCodeStrategy: hmacStrat,
		Storage:               store,
		Config:                config,
	}
	config.AuthorizeEndpointHandlers.Append(pkceHandler)
	config.TokenEndpointHandlers.Append(pkceHandler)

	// OIDC: ID tokens on authorize + token endpoints.
	oidcHandler := &openid.OpenIDConnectExplicitHandler{
		OpenIDConnectRequestStorage: store,
		IDTokenHandleHelper: &openid.IDTokenHandleHelper{
			IDTokenStrategy: oidcStrategy,
		},
		OpenIDConnectRequestValidator: openid.NewOpenIDConnectRequestValidator(jwtSigner, config),
		Config:                        config,
	}
	config.AuthorizeEndpointHandlers.Append(oidcHandler)
	config.TokenEndpointHandlers.Append(oidcHandler)

	// New ID token on refresh.
	config.TokenEndpointHandlers.Append(&openid.OpenIDConnectRefreshHandler{
		IDTokenHandleHelper: &openid.IDTokenHandleHelper{
			IDTokenStrategy: oidcStrategy,
		},
		Config: config,
	})

	// Revocation endpoint.
	config.RevocationHandlers.Append(&oauth2.TokenRevocationHandler{
		TokenRevocationStorage: store,
		AccessTokenStrategy:    hmacStrat,
		RefreshTokenStrategy:   hmacStrat,
	})

	// Introspection (used by /userinfo).
	config.TokenIntrospectionHandlers.Append(&oauth2.CoreValidator{
		CoreStrategy: hmacStrat,
		CoreStorage:  store,
		Config:       config,
	})

	return fosite.NewOAuth2Provider(store, config), nil
}

// Session is the fosite session carried through authorize → token.
// Subject is the PERSONA ID — the contract with Zeal products.
type Session struct {
	fosite.DefaultSession
	Claims  *jwt.IDTokenClaims `json:"claims,omitempty"`
	Headers *jwt.Headers       `json:"headers,omitempty"`
	Extra   map[string]any     `json:"extra,omitempty"`
}

// IDTokenClaims implements fosite/reuse.Session.
func (s *Session) IDTokenClaims() *jwt.IDTokenClaims {
	if s.Claims == nil {
		s.Claims = &jwt.IDTokenClaims{RequestedAt: time.Now().UTC()}
	}
	return s.Claims
}

// IDTokenHeaders implements fosite/reuse.Session.
func (s *Session) IDTokenHeaders() *jwt.Headers {
	if s.Headers == nil {
		s.Headers = &jwt.Headers{}
	}
	return s.Headers
}

// Clone implements fosite.Session.
func (s *Session) Clone() fosite.Session {
	c := &Session{Extra: make(map[string]any, len(s.Extra))}
	c.Subject = s.Subject
	c.Username = s.Username
	if s.ExpiresAt != nil {
		c.ExpiresAt = make(map[fosite.TokenType]time.Time, len(s.ExpiresAt))
		for k, v := range s.ExpiresAt {
			c.ExpiresAt[k] = v
		}
	}
	if s.Claims != nil {
		clone := *s.Claims
		c.Claims = &clone
	}
	if s.Headers != nil {
		clone := *s.Headers
		c.Headers = &clone
	}
	for k, v := range s.Extra {
		c.Extra[k] = v
	}
	return c
}

// NewSession builds the session for a persona: `sub` = persona ID,
// plus first-party zeal_* claims.
func NewSession(personaID, accountID, personaName string, identity *string, identityVerified bool) *Session {
	identityVal := any(nil)
	if identity != nil {
		identityVal = *identity
	}
	extra := map[string]any{
		"zeal_account_id":                accountID,
		"zeal_persona_name":              personaName,
		"zeal_persona_identity":          identityVal,
		"zeal_persona_identity_verified": identityVerified,
	}
	return &Session{
		DefaultSession: fosite.DefaultSession{
			Subject: personaID,
		},
		Claims: &jwt.IDTokenClaims{
			Subject:     personaID,
			RequestedAt: time.Now().UTC(),
			Extra:       extra,
		},
		Extra: extra,
	}
}
