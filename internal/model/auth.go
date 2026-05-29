package model

// AuthType selects which authentication scheme a Request, Folder, or
// Collection applies. "" is the zero value and is treated as Inherit at
// load time so a freshly created request defaults to inheriting from its
// parent without the caller having to set anything explicitly.
type AuthType string

// Supported authentication types. Inherit walks up the folder → collection
// tree at send time to find the nearest non-Inherit value; the collection
// root defaults to None because it has no parent to inherit from.
const (
	AuthNone    AuthType = "none"
	AuthInherit AuthType = "inherit"
	AuthBasic   AuthType = "basic"
	AuthBearer  AuthType = "bearer"
	AuthAPIKey  AuthType = "apikey"
	AuthOAuth2  AuthType = "oauth2"
)

// APIKeyPlacement chooses whether the API-Key credential rides on a request
// header or a query-string parameter.
type APIKeyPlacement string

// Where an API-Key credential is attached to the outgoing request.
const (
	APIKeyHeader APIKeyPlacement = "header"
	APIKeyQuery  APIKeyPlacement = "query"
)

// OAuth2Grant selects which OAuth2 grant flow Helena runs when fetching a
// token. v0.2 ships client_credentials and authorization_code (with PKCE);
// other grants are not yet implemented.
type OAuth2Grant string

// Supported OAuth2 grant types.
const (
	OAuth2ClientCredentials OAuth2Grant = "client_credentials"
	OAuth2AuthorizationCode OAuth2Grant = "authorization_code"
)

// Auth describes the authentication applied to a Request (or inherited from
// a Folder/Collection). Type selects which of the optional sub-structs is
// in use; the others are nil. The struct is a value, not a pointer, so the
// Request/Folder/Collection containers stay copyable.
type Auth struct {
	Type   AuthType    `json:"type,omitempty"`
	Basic  *BasicAuth  `json:"basic,omitempty"`
	Bearer *BearerAuth `json:"bearer,omitempty"`
	APIKey *APIKeyAuth `json:"apiKey,omitempty"`
	OAuth2 *OAuth2Auth `json:"oauth2,omitempty"`
}

// BasicAuth carries HTTP Basic credentials. Username and Password are run
// through the variable resolver at send time so `{{TOKEN}}` style
// indirection is supported.
type BasicAuth struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// BearerAuth carries a Bearer token that becomes
// `Authorization: Bearer <token>` on the outgoing request. The token is
// resolved through `{{var}}` substitution before being applied.
type BearerAuth struct {
	Token string `json:"token"`
}

// APIKeyAuth carries an API key plus its placement (header or query). Both
// the name and value run through the variable resolver before being added
// to the request.
type APIKeyAuth struct {
	Name      string          `json:"name"`
	Value     string          `json:"value"`
	Placement APIKeyPlacement `json:"placement,omitempty"`
}

// OAuth2Auth carries the configuration for an OAuth2 grant flow. Fields
// that don't apply to the selected Grant are ignored. UsePKCE applies only
// to authorization_code.
type OAuth2Auth struct {
	Grant        OAuth2Grant `json:"grant"`
	TokenURL     string      `json:"tokenUrl"`
	AuthURL      string      `json:"authUrl,omitempty"`
	ClientID     string      `json:"clientId"`
	ClientSecret string      `json:"clientSecret"`
	Scope        string      `json:"scope,omitempty"`
	RedirectURI  string      `json:"redirectUri,omitempty"`
	UsePKCE      bool        `json:"usePkce,omitempty"`
	Audience     string      `json:"audience,omitempty"`
}
