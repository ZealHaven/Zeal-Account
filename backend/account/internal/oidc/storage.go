// Package oidc implements the OIDC provider on top of fosite: storage,
// key management, and endpoint handlers. Persona is the `sub`.
package oidc

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ory/fosite"
	"github.com/ory/fosite/handler/oauth2"
	"github.com/ory/fosite/handler/openid"
	"github.com/ory/fosite/handler/pkce"
)

// Storage is the Postgres-backed fosite store.
// Adapted from the verified Zeal/Auth implementation (prior art).
type Storage struct {
	db *pgxpool.Pool
}

// NewStorage wires the store.
func NewStorage(db *pgxpool.Pool) *Storage { return &Storage{db: db} }

// Ensure Storage implements every fosite interface the flows need.
var (
	_ fosite.Storage                     = (*Storage)(nil)
	_ fosite.ClientManager               = (*Storage)(nil)
	_ oauth2.CoreStorage                 = (*Storage)(nil)
	_ oauth2.TokenRevocationStorage      = (*Storage)(nil)
	_ pkce.PKCERequestStorage            = (*Storage)(nil)
	_ openid.OpenIDConnectRequestStorage = (*Storage)(nil)
)

// storedRequest is a serializable fosite.Request.
type storedRequest struct {
	ID                string                `json:"id"`
	RequestedAt       time.Time             `json:"requested_at"`
	Client            *fosite.DefaultClient `json:"client"`
	RequestedScope    []string              `json:"requested_scope"`
	GrantedScope      []string              `json:"granted_scope"`
	Form              map[string][]string   `json:"form"`
	SessionData       json.RawMessage       `json:"session_data"`
	RequestedAudience []string              `json:"requested_audience"`
	GrantedAudience   []string              `json:"granted_audience"`
	// Invalidated marks a single-use authorize code as redeemed (or a
	// rotated refresh token) without deleting it, so replays are detected.
	Invalidated bool `json:"invalidated,omitempty"`
}

func requestToStored(req fosite.Requester) *storedRequest {
	sr := &storedRequest{
		ID:                req.GetID(),
		RequestedAt:       req.GetRequestedAt(),
		RequestedScope:    []string(req.GetRequestedScopes()),
		GrantedScope:      []string(req.GetGrantedScopes()),
		RequestedAudience: []string(req.GetRequestedAudience()),
		GrantedAudience:   []string(req.GetGrantedAudience()),
	}
	form := req.GetRequestForm()
	sr.Form = make(map[string][]string, len(form))
	for k, v := range form {
		sr.Form[k] = v
	}
	if c := req.GetClient(); c != nil {
		sr.Client = &fosite.DefaultClient{
			ID:            c.GetID(),
			Secret:        c.GetHashedSecret(),
			RedirectURIs:  c.GetRedirectURIs(),
			GrantTypes:    []string(c.GetGrantTypes()),
			ResponseTypes: []string(c.GetResponseTypes()),
			Scopes:        []string(c.GetScopes()),
			Audience:      []string(c.GetAudience()),
			Public:        c.IsPublic(),
		}
	}
	return sr
}

func (sr *storedRequest) toRequest(session fosite.Session) *fosite.Request {
	req := &fosite.Request{
		ID:                sr.ID,
		RequestedAt:       sr.RequestedAt,
		Client:            sr.Client,
		RequestedScope:    fosite.Arguments(sr.RequestedScope),
		GrantedScope:      fosite.Arguments(sr.GrantedScope),
		Form:              make(map[string][]string),
		Session:           session,
		RequestedAudience: fosite.Arguments(sr.RequestedAudience),
		GrantedAudience:   fosite.Arguments(sr.GrantedAudience),
	}
	for k, v := range sr.Form {
		req.Form[k] = v
	}
	if session != nil && len(sr.SessionData) > 0 {
		_ = json.Unmarshal(sr.SessionData, session)
	}
	return req
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", h)
}

func (s *Storage) storeRequest(ctx context.Context, key string, req fosite.Requester) error {
	sr := requestToStored(req)
	if req.GetSession() != nil {
		data, err := json.Marshal(req.GetSession())
		if err != nil {
			return fmt.Errorf("marshal session: %w", err)
		}
		sr.SessionData = data
	}
	raw, err := json.Marshal(sr)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO fosite_requests (key, data) VALUES ($1, $2)
		ON CONFLICT (key) DO UPDATE SET data = $2
	`, key, raw)
	return err
}

func (s *Storage) getRequest(ctx context.Context, key string, session fosite.Session) (*fosite.Request, error) {
	var data []byte
	err := s.db.QueryRow(ctx, `SELECT data FROM fosite_requests WHERE key = $1`, key).Scan(&data)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fosite.ErrNotFound
		}
		return nil, err
	}
	var sr storedRequest
	if err := json.Unmarshal(data, &sr); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	if sr.Client == nil {
		sr.Client = &fosite.DefaultClient{}
	}
	return sr.toRequest(session), nil
}

func (s *Storage) deleteRequest(ctx context.Context, key string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM fosite_requests WHERE key = $1`, key)
	return err
}

// Invalidate flips the invalidated flag on a stored request.
func (s *Storage) invalidateRequest(ctx context.Context, key string) error {
	var data []byte
	err := s.db.QueryRow(ctx, `SELECT data FROM fosite_requests WHERE key = $1`, key).Scan(&data)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fosite.ErrNotFound
		}
		return err
	}
	var sr storedRequest
	if err := json.Unmarshal(data, &sr); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}
	sr.Invalidated = true
	raw, err := json.Marshal(&sr)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	_, err = s.db.Exec(ctx, `UPDATE fosite_requests SET data = $2 WHERE key = $1`, key, raw)
	return err
}

// ---- ClientManager ----

// GetClient loads a first-party OAuth client.
func (s *Storage) GetClient(ctx context.Context, id string) (fosite.Client, error) {
	var secretHash *string
	var name, status string
	var scopes []string
	var isPublic bool
	err := s.db.QueryRow(ctx, `
		SELECT secret_hash, name, status, scopes, is_public
		FROM oauth_clients WHERE id = $1
	`, id).Scan(&secretHash, &name, &status, &scopes, &isPublic)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fosite.ErrNotFound
		}
		return nil, err
	}
	if status != "active" {
		return nil, fosite.ErrNotFound
	}
	uris, err := s.redirectURIs(ctx, id)
	if err != nil {
		return nil, err
	}
	// Public clients authenticate with PKCE only; confidential clients
	// additionally send a client_secret (stored hashed).
	var hashed []byte
	if secretHash != nil {
		hashed = []byte(*secretHash)
	}
	return &fosite.DefaultClient{
		ID:            id,
		Secret:        hashed,
		RedirectURIs:  uris,
		GrantTypes:    fosite.Arguments{"authorization_code", "refresh_token"},
		ResponseTypes: fosite.Arguments{"code"},
		Scopes:        fosite.Arguments(scopes),
		Audience:      fosite.Arguments{id},
		Public:        isPublic,
	}, nil
}

func (s *Storage) redirectURIs(ctx context.Context, clientID string) ([]string, error) {
	rows, err := s.db.Query(ctx, `
		SELECT redirect_uri FROM oauth_client_redirect_uris WHERE client_id = $1
	`, clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// ClientAssertionJWTValid reports whether a client-assertion JTI was seen
// (replay ⇒ invalid). Unused in MVP (no private_key_jwt clients yet).
func (s *Storage) ClientAssertionJWTValid(ctx context.Context, jti string) error {
	var exists bool
	if err := s.db.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM oauth_client_assertions WHERE jti = $1)
	`, jti).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return fosite.ErrNotFound
	}
	return nil
}

// SetClientAssertionJWT records a JTI and prunes expired ones.
func (s *Storage) SetClientAssertionJWT(ctx context.Context, jti string, exp time.Time) error {
	if _, err := s.db.Exec(ctx, `
		INSERT INTO oauth_client_assertions (jti, expires_at)
		VALUES ($1, $2) ON CONFLICT (jti) DO NOTHING
	`, jti, exp); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, `DELETE FROM oauth_client_assertions WHERE expires_at < now()`)
	return err
}

// ---- Authorize codes ----

// CreateAuthorizeCodeSession stores the code's request.
func (s *Storage) CreateAuthorizeCodeSession(ctx context.Context, code string, request fosite.Requester) error {
	return s.storeRequest(ctx, "authcode:"+hashToken(code), request)
}

// GetAuthorizeCodeSession loads it; invalidated ⇒ replay detected.
func (s *Storage) GetAuthorizeCodeSession(ctx context.Context, code string, session fosite.Session) (fosite.Requester, error) {
	key := "authcode:" + hashToken(code)
	var data []byte
	if err := s.db.QueryRow(ctx, `SELECT data FROM fosite_requests WHERE key = $1`, key).Scan(&data); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fosite.ErrNotFound
		}
		return nil, err
	}
	var sr storedRequest
	if err := json.Unmarshal(data, &sr); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	if sr.Client == nil {
		sr.Client = &fosite.DefaultClient{}
	}
	req := sr.toRequest(session)
	if sr.Invalidated {
		return req, fosite.ErrInvalidatedAuthorizeCode
	}
	return req, nil
}

// InvalidateAuthorizeCodeSession marks a code redeemed (single-use).
func (s *Storage) InvalidateAuthorizeCodeSession(ctx context.Context, code string) error {
	return s.invalidateRequest(ctx, "authcode:"+hashToken(code))
}

// ---- Access tokens ----

// CreateAccessTokenSession stores the token session + revocation row.
func (s *Storage) CreateAccessTokenSession(ctx context.Context, signature string, request fosite.Requester) error {
	if err := s.storeRequest(ctx, "accesstoken:"+signature, request); err != nil {
		return err
	}
	var expiresAt *time.Time
	if sess := request.GetSession(); sess != nil {
		if exp := sess.GetExpiresAt(fosite.AccessToken); !exp.IsZero() {
			expiresAt = &exp
		}
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO oauth_access_tokens (signature, request_id, expires_at)
		VALUES ($1, $2, $3) ON CONFLICT (signature) DO NOTHING
	`, signature, request.GetID(), expiresAt)
	return err
}

// GetAccessTokenSession loads it unless revoked.
func (s *Storage) GetAccessTokenSession(ctx context.Context, signature string, session fosite.Session) (fosite.Requester, error) {
	var revokedAt *time.Time
	err := s.db.QueryRow(ctx, `
		SELECT revoked_at FROM oauth_access_tokens WHERE signature = $1
	`, signature).Scan(&revokedAt)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if revokedAt != nil {
		return nil, fosite.ErrNotFound
	}
	return s.getRequest(ctx, "accesstoken:"+signature, session)
}

// DeleteAccessTokenSession removes the token session.
func (s *Storage) DeleteAccessTokenSession(ctx context.Context, signature string) error {
	return s.deleteRequest(ctx, "accesstoken:"+signature)
}

// ---- Refresh tokens ----

// CreateRefreshTokenSession stores the refresh token session.
func (s *Storage) CreateRefreshTokenSession(ctx context.Context, signature, accessSignature string, request fosite.Requester) error {
	return s.storeRequest(ctx, "refreshtoken:"+signature, request)
}

// GetRefreshTokenSession loads it; invalidated ⇒ rotation reuse detected.
func (s *Storage) GetRefreshTokenSession(ctx context.Context, signature string, session fosite.Session) (fosite.Requester, error) {
	key := "refreshtoken:" + signature
	var data []byte
	if err := s.db.QueryRow(ctx, `SELECT data FROM fosite_requests WHERE key = $1`, key).Scan(&data); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fosite.ErrNotFound
		}
		return nil, err
	}
	var sr storedRequest
	if err := json.Unmarshal(data, &sr); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	if sr.Client == nil {
		sr.Client = &fosite.DefaultClient{}
	}
	req := sr.toRequest(session)
	if sr.Invalidated {
		// Replay of a rotated token: keep the request for family revocation.
		return req, fosite.ErrInactiveToken
	}
	return req, nil
}

// DeleteRefreshTokenSession removes the refresh token session.
func (s *Storage) DeleteRefreshTokenSession(ctx context.Context, signature string) error {
	return s.deleteRequest(ctx, "refreshtoken:"+signature)
}

// RotateRefreshToken marks the old token rotated (kept for reuse detection).
func (s *Storage) RotateRefreshToken(ctx context.Context, requestID, refreshTokenSignature string) error {
	return s.invalidateRequest(ctx, "refreshtoken:"+refreshTokenSignature)
}

// RevokeRefreshToken deletes all refresh sessions of a request.
func (s *Storage) RevokeRefreshToken(ctx context.Context, requestID string) error {
	_, _ = s.db.Exec(ctx, `
		DELETE FROM fosite_requests
		WHERE key LIKE 'refreshtoken:%' AND data::jsonb->>'id' = $1
	`, requestID)
	return nil
}

// RevokeAccessToken revokes all access tokens of a request.
func (s *Storage) RevokeAccessToken(ctx context.Context, requestID string) error {
	_, _ = s.db.Exec(ctx, `
		UPDATE oauth_access_tokens SET revoked_at = now()
		WHERE request_id = $1 AND revoked_at IS NULL
	`, requestID)
	_, _ = s.db.Exec(ctx, `
		DELETE FROM fosite_requests
		WHERE key LIKE 'accesstoken:%' AND data::jsonb->>'id' = $1
	`, requestID)
	return nil
}

// ---- PKCE ----

// CreatePKCERequestSession stores PKCE verifier state.
func (s *Storage) CreatePKCERequestSession(ctx context.Context, signature string, request fosite.Requester) error {
	return s.storeRequest(ctx, "pkce:"+signature, request)
}

// GetPKCERequestSession loads PKCE verifier state.
func (s *Storage) GetPKCERequestSession(ctx context.Context, signature string, session fosite.Session) (fosite.Requester, error) {
	return s.getRequest(ctx, "pkce:"+signature, session)
}

// DeletePKCERequestSession removes PKCE verifier state.
func (s *Storage) DeletePKCERequestSession(ctx context.Context, signature string) error {
	return s.deleteRequest(ctx, "pkce:"+signature)
}

// ---- OIDC ----

// CreateOpenIDConnectSession stores ID-token state for the code.
func (s *Storage) CreateOpenIDConnectSession(ctx context.Context, authorizeCode string, request fosite.Requester) error {
	return s.storeRequest(ctx, "oidcsession:"+hashToken(authorizeCode), request)
}

// GetOpenIDConnectSession hydrates the caller's session with stored claims.
func (s *Storage) GetOpenIDConnectSession(ctx context.Context, authorizeCode string, request fosite.Requester) (fosite.Requester, error) {
	return s.getRequest(ctx, "oidcsession:"+hashToken(authorizeCode), request.GetSession())
}

// DeleteOpenIDConnectSession removes ID-token state for the code.
func (s *Storage) DeleteOpenIDConnectSession(ctx context.Context, authorizeCode string) error {
	return s.deleteRequest(ctx, "oidcsession:"+hashToken(authorizeCode))
}
