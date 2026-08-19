# OIDC / OAuth 2.0 Provider

GenID ships an embedded OIDC provider (RS256 signed JWTs) used for token minting, the dev-login flow, and third-party client integration.

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/.well-known/openid-configuration` | Discovery document (issuer, authorization/token/userinfo/jwks/introspect/revoke/device endpoints, scopes, grant types, PKCE S256) |
| GET | `/.well-known/jwks.json` | RSA JWKS (`kty=RSA`, `alg=RS256`, `use=sig`) |
| GET/POST | `/authorize` | Authorization request: `client_id, redirect_uri, response_type=code, scope, state, code_challenge, code_challenge_method=S256, nonce`. PKCE codes stored in `oidc_auth_codes` (5 min TTL, single-use). |
| POST | `/token` | Token grant. Supports `authorization_code` (PKCE verified), `refresh_token` (rotation), `client_credentials`, and `urn:ietf:params:oauth:grant-type:device_code`. Issues RS256 access token + optional id_token. |
| GET | `/userinfo` | Current user claims (`sub, email, name, preferred_username, department`) |
| POST | `/introspect` | Introspect an access/refresh token |
| POST | `/revoke` | Revoke refresh token (hashed at rest) |
| POST | `/device_authorization` | Device flow: create `DeviceCodeEntry` (15 min TTL) |
| GET/POST | `/device` | Device verification (user approves `user_code`) |

## Client management

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/oidc/clients` | List clients → `{"clients":[ClientRecord]}` |
| POST | `/api/v1/oidc/clients` | Create `{"name","redirect_uris","grant_types","is_public"}` → `201` `{id, name, client_id:"oidc-<name>-<8hex>", client_secret:"secret-<hex>", redirect_uris, grant_types, scopes, is_public, created_at}`. Defaults: grants `authorization_code refresh_token`, scopes `openid profile email`. |
| DELETE | `/api/v1/oidc/clients/{client_id}` | Delete → `{"status":"deleted"}` |

## Scopes & grants

| Scope | Grants |
|-------|--------|
| `openid profile email offline_access api` | `authorization_code client_credentials refresh_token urn:ietf:params:oauth:grant-type:device_code` |

## Tokens

- **Access token**: RS256 JWT. Claims include `sub` (identity UUID), `iss`, `aud`, `iat`, `exp`, `roles`, `scope`, `tenant_id`.
- **id_token**: RS256, claims `sub, iss, aud, iat, exp, email, name, preferred_username, department`.
- **Refresh token**: opaque, stored hashed in `oidc_refresh_tokens`, rotated on use.

## Try it (test console)

The UI exposes an **IDP test console** at `/idp` (direct URL): create a client, run the authorize/token/userinfo flow end-to-end.

## Related

- [Auth overview](overview.md#authentication)
- [OIDC data model](../data-model/postgres.md#oidc)
