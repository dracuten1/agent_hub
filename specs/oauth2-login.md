# OAuth2 Login (Google + GitHub) — Design Specification

> **Status:** Draft  
> **Date:** 2026-04-03  
> **Project:** `/root/.openclaw/workspace-pm/projects/agenthub/`  
> **Build:** `export PATH=$PATH:/usr/local/go/bin && go build ./...`

---

## 1. Overview

Add OAuth2 login with Google and GitHub. Users can sign in with either provider, link multiple providers to one account, and see linked providers on a profile page.

**Constraints:**
- Go 1.21, Gin, PostgreSQL 15, sqlx
- Existing username/password auth stays untouched
- OAuth is optional — no breaking changes
- Frontend: React + Vite, CSS-only (no UI libraries)
- `golang.org/x/oauth2` for OAuth2 flows (standard library ecosystem)

---

## 2. Architecture

```
Browser                     Go Backend                     Provider
  │                              │                              │
  │  1. Click "Login with Google"│                              │
  │─────────────────────────────>│                              │
  │  2. 302 → provider auth URL  │                              │
  │<─────────────────────────────│                              │
  │                              │                              │
  │  3. User authenticates       │                              │
  │─────────────────────────────────────────────────────────────>│
  │                              │                              │
  │  4. 302 → /api/auth/callback?code=...&state=...             │
  │<─────────────────────────────────────────────────────────────│
  │─────────────────────────────>│                              │
  │                              │  5. Exchange code for token  │
  │                              │─────────────────────────────>│
  │                              │  6. Get user profile         │
  │                              │─────────────────────────────>│
  │                              │                              │
  │                              │  7. Find or create user      │
  │                              │  8. Generate JWT             │
  │  9. 302 → /?token=jwt        │                              │
  │<─────────────────────────────│                              │
  │                              │                              │
  │  10. Store token, show app   │                              │
```

**Flow:**
1. Backend generates OAuth2 authorization URL with random `state` (CSRF protection)
2. Redirect browser to provider
3. Provider redirects back to `/api/auth/:provider/callback`
4. Backend exchanges code → access token → provider user info
5. Backend looks up user by provider ID in `oauth_accounts` table
6. If found → login (generate JWT). If not → auto-create user (username = provider name, no password)
7. Redirect to frontend with JWT in query param
8. Frontend stores JWT in localStorage

---

## 3. Data Model Changes

### New Table: `oauth_accounts`

```sql
-- 011_oauth.sql
CREATE TABLE IF NOT EXISTS oauth_accounts (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider    VARCHAR(20) NOT NULL,          -- 'google' or 'github'
    provider_id TEXT NOT NULL,                 -- provider's user ID
    email       VARCHAR(200),                  -- email from provider
    avatar_url  TEXT,                          -- profile picture URL
    access_token TEXT,                         -- encrypted refresh token (future use)
    created_at  TIMESTAMP DEFAULT NOW(),
    updated_at  TIMESTAMP DEFAULT NOW(),
    UNIQUE(provider, provider_id)
);

CREATE INDEX IF NOT EXISTS idx_oauth_user ON oauth_accounts(user_id);
CREATE INDEX IF NOT EXISTS idx_oauth_provider ON oauth_accounts(provider, provider_id);
```

### Modify: `users` table

```sql
-- Make password nullable for OAuth-only users
ALTER TABLE users ALTER COLUMN password DROP NOT NULL;

-- Add avatar_url column
ALTER TABLE users ADD COLUMN IF NOT EXISTS avatar_url TEXT;
```

### Modify: `users` table — auth_source

```sql
ALTER TABLE users ADD COLUMN IF NOT EXISTS auth_source VARCHAR(20) DEFAULT 'local';
-- Values: 'local', 'google', 'github', 'linked'
```

---

## 4. API Endpoints

### 4.1 Initiate OAuth Login

```
GET /api/auth/google/login
GET /api/auth/github/login
```

**Response:** `302 Redirect` to provider authorization URL.

**Behavior:**
1. Generate random `state` (32 bytes hex), store in cookie `oauth_state` (httpOnly, secure, sameSite=lax, maxAge=600)
2. Build authorization URL with: `client_id`, `redirect_uri`, `scope`, `state`
3. Redirect 302

**Scopes:**
- Google: `openid`, `email`, `profile`
- GitHub: `user:email`, `read:user`

### 4.2 OAuth Callback

```
GET /api/auth/google/callback?code=...&state=...
GET /api/auth/github/callback?code=...&state=...
```

**Response:** `302 Redirect` to frontend `{FRONTEND_URL}/?token={jwt}&provider={name}`

**Behavior:**
1. Validate `state` param matches `oauth_state` cookie. Clear cookie after check. Reject on mismatch (400).
2. Exchange `code` for access token via provider's token endpoint.
3. Fetch user profile from provider's user info endpoint.
4. Look up `oauth_accounts` by `(provider, provider_id)`.
5. **If found:** get associated `user_id`, generate JWT, redirect with token.
6. **If not found:** auto-create user:
   - `username`: provider username (append random suffix if taken)
   - `email`: provider email (append random suffix if taken)
   - `password`: NULL (OAuth-only user)
   - `avatar_url`: from provider
   - `auth_source`: provider name
   - Create `oauth_accounts` row linking to new user
   - Generate JWT, redirect with token
7. Update `users.avatar_url` and `oauth_accounts.avatar_url` on every login.

**Error handling:**
- Invalid state → 400 `Invalid OAuth state`
- Code exchange fails → 502 `Provider error: {details}`
- Provider returns error → 400 `OAuth error: {provider_error}`

### 4.3 Link Provider to Existing Account

```
POST /api/auth/link
Authorization: Bearer {jwt}

{
  "provider": "google",    // or "github"
  "code": "...",           // OAuth authorization code
  "state": "..."           // OAuth state from cookie
}
```

**Response:** `200 {linked: true, provider: "google"}`

**Behavior:**
1. Validate JWT, get `userID` from context.
2. Validate `state` from cookie.
3. Exchange code → token → profile.
4. Check if provider account already linked to any user → 409 `Provider account already linked`.
5. Insert `oauth_accounts` row.
6. Update `users.auth_source = 'linked'`.
7. Return 200.

### 4.4 Unlink Provider

```
DELETE /api/auth/link/:provider
Authorization: Bearer {jwt}
```

**Response:** `200 {unlinked: true}`

**Behavior:**
1. Validate JWT, get `userID`.
2. Delete from `oauth_accounts` where `user_id = userID AND provider = :provider`.
3. If user has no more linked accounts and `password IS NULL` → 400 `Cannot unlink last sign-in method`.
4. Return 200.

### 4.5 Get Profile (with linked accounts)

```
GET /api/auth/profile
Authorization: Bearer {jwt}
```

**Response:**
```json
{
  "id": "uuid",
  "username": "johndoe",
  "email": "john@example.com",
  "avatar_url": "https://...",
  "auth_source": "linked",
  "providers": [
    {"provider": "google", "email": "john@gmail.com", "avatar_url": "https://...", "linked_at": "..."},
    {"provider": "github", "email": "john@github.com", "avatar_url": "https://...", "linked_at": "..."}
  ]
}
```

### 4.6 Get Available Providers

```
GET /api/auth/providers
```

**Response:** `200` (public, no auth)
```json
{
  "providers": [
    {"id": "google", "name": "Google", "enabled": true},
    {"id": "github", "name": "GitHub", "enabled": true}
  ]
}
```

**Behavior:** Returns enabled providers based on whether `GOOGLE_CLIENT_ID` / `GITHUB_CLIENT_ID` env vars are set. Disabled providers have `enabled: false`.

---

## 5. Backend Implementation

### 5.1 New Package: `internal/oauth`

```
internal/oauth/
  provider.go   — Provider interface + Google/GitHub implementations
  handler.go    — HTTP handlers (login, callback, link, unlink, profile, providers)
  store.go      — Database operations for oauth_accounts
```

### 5.2 Provider Interface

```go
// internal/oauth/provider.go

type Provider interface {
    Name() string                                        // "google" or "github"
    AuthCodeURL(state string) string                     // provider authorization URL
    ExchangeCode(ctx context.Context, code string) (*Token, error)
    GetUserProfile(ctx context.Context, token *Token) (*UserProfile, error)
}

type Token struct {
    AccessToken  string
    RefreshToken string
    Expiry       time.Time
}

type UserProfile struct {
    ProviderID string // provider's unique user ID
    Username   string
    Email      string
    AvatarURL  string
}
```

### 5.3 Google Provider

```go
// Uses golang.org/x/oauth2 + google endpoint
// Token URL:    https://oauth2.googleapis.com/token
// Auth URL:     https://accounts.google.com/o/oauth2/v2/auth
// Profile URL:  https://www.googleapis.com/oauth2/v2/userinfo
// Response:     {"id": "...", "email": "...", "name": "...", "picture": "..."}
```

### 5.4 GitHub Provider

```go
// Token URL:    https://github.com/login/oauth/access_token
// Auth URL:     https://github.com/login/oauth/authorize
// Profile URL:  https://api.github.com/user
// Response:     {"id": 123, "login": "...", "email": "...", "avatar_url": "..."}
```

### 5.5 Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `GOOGLE_CLIENT_ID` | No | "" | Google OAuth client ID |
| `GOOGLE_CLIENT_SECRET` | No | "" | Google OAuth client secret |
| `GITHUB_CLIENT_ID` | No | "" | GitHub OAuth client ID |
| `GITHUB_CLIENT_SECRET` | No | "" | GitHub OAuth client secret |
| `OAUTH_REDIRECT_BASE` | No | `http://localhost:8081` | Base URL for callbacks |
| `FRONTEND_URL` | No | `http://localhost:5173` | Redirect target after login |

Provider is **disabled** if its `CLIENT_ID` is empty.

### 5.6 Route Registration

```go
// cmd/server/main.go
oauthHandler := oauth.NewHandler(db, jwtSecret, providers)
public.GET("/auth/providers", oauthHandler.GetProviders)
public.GET("/auth/google/login", oauthHandler.Login("google"))
public.GET("/auth/github/login", oauthHandler.Login("github"))
public.GET("/auth/google/callback", oauthHandler.Callback("google"))
public.GET("/auth/github/callback", oauthHandler.Callback("github"))
user.GET("/auth/profile", oauthHandler.GetProfile)
user.POST("/auth/link", oauthHandler.LinkProvider)
user.DELETE("/auth/link/:provider", oauthHandler.UnlinkProvider)
```

---

## 6. Frontend Implementation

### 6.1 Changes to Login.jsx

Add OAuth buttons below the existing login form:

```
┌─────────────────────────────┐
│        AgentHub              │
│                              │
│  [Username_____________]     │
│  [Password_____________]     │
│  [       Login        ]     │
│                              │
│  ─── or continue with ───   │
│                              │
│  [G] Google    [🐙] GitHub  │
│                              │
│  Don't have an account?      │
│  Register                    │
└─────────────────────────────┘
```

- Call `GET /api/auth/providers` on mount to check which providers are enabled
- Only show enabled provider buttons
- Clicking a button: `window.location.href = '/api/auth/google/login'`
- Full-page redirect (not SPA navigation) — browser goes to provider and back

### 6.2 New: Profile.jsx

```
┌─────────────────────────────────────┐
│ 🤖 AgentHub    Dashboard  Board    │
│                        Profile  ⬤  │
├─────────────────────────────────────┤
│                                     │
│  ┌──────────┐                       │
│  │  Avatar   │  johndoe            │
│  │   (48px)  │  john@example.com   │
│  └──────────┘  Joined: Mar 2026    │
│                                     │
│  Linked Accounts                    │
│  ┌─────────────────────────────┐   │
│  │ [G] Google  john@gmail.com  ✅ │   │
│  │    Linked Mar 15, 2026      🗑️ │   │
│  ├─────────────────────────────┤   │
│  │ [🐙] GitHub  johndoe       ✅ │   │
│  │    Linked Mar 20, 2026      🗑️ │   │
│  └─────────────────────────────┘   │
│                                     │
│  Security                           │
│  Password: Set (Change)             │
│  Two-factor: Not enabled            │
│                                     │
└─────────────────────────────────────┘
```

### 6.3 Changes to App.jsx

- Add `Profile` tab in navbar
- Add OAuth callback detection in `useEffect`:
  ```js
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const token = params.get('token');
    if (token) {
      setToken(token);
      // Parse JWT payload to get user info
      const payload = JSON.parse(atob(token.split('.')[1]));
      setUser({ username: payload.username, id: payload.sub });
      // Clean URL
      window.history.replaceState({}, '', '/');
    }
  }, []);
  ```

### 6.4 Changes to api/client.js

```js
export const auth = {
  // ... existing register, login ...
  providers: () => request('/api/auth/providers'),
  profile: () => request('/api/auth/profile'),
  link: (provider, code, state) =>
    request('/api/auth/link', {
      method: 'POST',
      body: JSON.stringify({ provider, code, state }),
    }),
  unlink: (provider) =>
    request(`/api/auth/link/${provider}`, { method: 'DELETE' }),
};
```

---

## 7. Security Considerations

1. **CSRF via `state` parameter:** Random 32-byte hex stored in httpOnly cookie, validated on callback, single-use.
2. **PKCE not needed:** Server-side code exchange (not public client). Backend holds `client_secret`.
3. **Redirect URL validation:** `OAUTH_REDIRECT_BASE` must match what's configured in Google/GitHub console.
4. **Token in URL:** JWT is in query param for one redirect only. Frontend immediately stores in localStorage and strips from URL. Short-lived token (24h).
5. **Email uniqueness:** OAuth email may differ from registered email. Don't auto-merge accounts by email — explicit link action required.
6. **Account enumeration:** Login endpoints return generic errors. Callback failures return generic redirect to `/login?error=oauth_failed`.

---

## 8. Implementation Tasks

### Task Breakdown

| # | Task | Assignee | Est. Time | Dependencies |
|---|------|----------|-----------|--------------|
| T1 | Add migration + oauth_accounts table | dev2 | 15 min | None |
| T2 | Create `internal/oauth` package (provider interface, Google, GitHub, store) | dev2 | 45 min | T1 |
| T3 | Add OAuth HTTP handlers + register routes | dev2 | 30 min | T2 |
| T4 | Add callback detection in App.jsx + provider buttons in Login.jsx | dev1 | 20 min | None |
| T5 | Create Profile.jsx + profile API in client.js | dev1 | 30 min | T4 |
| T6 | Update navbar with Profile tab | dev1 | 10 min | T5 |

**Parallelization:**
- Dev1: T4 → T5 → T6 (frontend)
- Dev2: T1 → T2 → T3 (backend)

**Total wall time:** ~1.5 hours

---

## 9. Files Summary

### Files to Create
| File | Task |
|------|------|
| `internal/oauth/provider.go` | T2 |
| `internal/oauth/handler.go` | T3 |
| `internal/oauth/store.go` | T2 |
| `web/src/components/Profile.jsx` | T5 |

### Files to Modify
| File | Task | Changes |
|------|------|---------|
| `internal/db/migrations.sql` | T1 | Add `oauth_accounts` table, alter `users` (nullable password, avatar_url, auth_source) |
| `cmd/server/main.go` | T3 | Register OAuth routes, configure providers from env |
| `web/src/App.jsx` | T4 | OAuth callback detection, Profile tab |
| `web/src/components/Login.jsx` | T4 | OAuth provider buttons |
| `web/src/api/client.js` | T5 | Add `auth.providers`, `auth.profile`, `auth.link`, `auth.unlink` |
| `web/src/index.css` | T4, T5 | OAuth button styles, profile page styles |

---

## 10. Acceptance Criteria

- [ ] `GET /api/auth/providers` returns enabled providers based on env vars
- [ ] `GET /api/auth/google/login` redirects to Google consent screen
- [ ] `GET /api/auth/github/login` redirects to GitHub authorization
- [ ] OAuth callback creates user on first login (no password required)
- [ ] OAuth callback logs in existing user on subsequent logins
- [ ] JWT is returned via redirect to frontend
- [ ] Frontend detects token in URL, stores it, strips from address bar
- [ ] Profile page shows linked providers with unlink option
- [ ] `POST /api/auth/link` links provider to existing account
- [ ] `DELETE /api/auth/link/:provider` unlinks provider (blocks if last method)
- [ ] Existing username/password login still works
- [ ] `go build ./...` passes

---

*Last updated: 2026-04-03*
