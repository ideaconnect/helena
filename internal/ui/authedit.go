package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/idct/helena/internal/model"
)

// authTypeLabels keeps a stable display order for the auth Type dropdown
// and maps human-friendly labels to model.AuthType values.
var authTypeLabels = []string{"None", "Inherit from parent", "Basic Auth", "Digest Auth", "NTLM", "Bearer Token", "API Key", "OAuth 1.0a", "OAuth 2.0", "WS-Security", "AWS Signature v4"}

const authInheritLabelText = "Inherit from parent"

var authTypeByLabel = map[string]model.AuthType{
	"None":               model.AuthNone,
	authInheritLabelText: model.AuthInherit,
	"Basic Auth":         model.AuthBasic,
	"Bearer Token":       model.AuthBearer,
	"API Key":            model.AuthAPIKey,
	"OAuth 1.0a":         model.AuthOAuth1,
	"OAuth 2.0":          model.AuthOAuth2,
	"WS-Security":        model.AuthWSSE,
	"AWS Signature v4":   model.AuthAWSV4,
	"Digest Auth":        model.AuthDigest,
	"NTLM":               model.AuthNTLM,
}

var authLabelByType = map[model.AuthType]string{
	model.AuthNone:    "None",
	model.AuthInherit: authInheritLabelText,
	model.AuthBasic:   "Basic Auth",
	model.AuthBearer:  "Bearer Token",
	model.AuthAPIKey:  "API Key",
	model.AuthOAuth1:  "OAuth 1.0a",
	model.AuthOAuth2:  "OAuth 2.0",
	model.AuthWSSE:    "WS-Security",
	model.AuthAWSV4:   "AWS Signature v4",
	model.AuthDigest:  "Digest Auth",
	model.AuthNTLM:    "NTLM",
}

var apiKeyPlacementLabels = []string{"Header", "Query"}

var oauth2GrantLabels = []string{"Client Credentials", "Authorization Code"}

var oauth2GrantByLabel = map[string]model.OAuth2Grant{
	"Client Credentials": model.OAuth2ClientCredentials,
	"Authorization Code": model.OAuth2AuthorizationCode,
}

var oauth2LabelByGrant = map[model.OAuth2Grant]string{
	model.OAuth2ClientCredentials: "Client Credentials",
	model.OAuth2AuthorizationCode: "Authorization Code",
}

// authEditorOpts configures an authEditor for its host: the request editor's
// Auth tab binds it to the current request, the folder / collection settings
// dialogs bind it to a working copy.
type authEditorOpts struct {
	// target returns the Auth being edited. A nil result drops the write (no
	// request loaded), mirroring the currentRequest == nil guard.
	target func() *model.Auth
	// onChange runs after every write-back (dirty tracking). Optional.
	onChange func()
	// inheritText produces the Inherit panel's preview line. Optional; the
	// panel shows a generic sentence when nil.
	inheritText func() string
	// busy is an extra write-back suppressor consulted on every callback —
	// the request tab passes m.loading so the editor honours the loadRequest
	// guard (AGENTS invariant 3). Optional.
	busy func() bool
	// allowInherit keeps "Inherit from parent" in the Type list. Off for the
	// collection root, which has no parent to inherit from.
	allowInherit bool
	// tokenNamespace scopes the OAuth2 "Clear cached tokens" button to one
	// collection's cache namespace (its directory).
	tokenNamespace func() string
}

// authEditor is the auth form shared by the request editor's Auth tab and the
// folder / collection settings dialogs: a Type dropdown plus a stack of
// per-scheme panels, only the one matching the selected Type visible. Every
// widget writes straight through to opts.target() — lazily allocating the
// matching sub-struct — unless load() is pushing values in or opts.busy says
// the host is.
type authEditor struct {
	m    *MainUI
	opts authEditorOpts
	// loading suppresses write-back while load() populates the widgets.
	loading bool

	typeSel *widget.Select

	basicUsername, basicPassword                             *shortcutEntry
	digestUsername, digestPassword                           *shortcutEntry
	ntlmUsername, ntlmPassword, ntlmDomain, ntlmWorkstation  *shortcutEntry
	bearerToken                                              *shortcutEntry
	wsseUsername, wssePassword                               *shortcutEntry
	oauth1ConsumerKey, oauth1ConsumerSecret                  *shortcutEntry
	oauth1Token, oauth1TokenSecret                           *shortcutEntry
	awsAccessKey, awsSecretKey, awsRegion                    *shortcutEntry
	awsService, awsSessionToken                              *shortcutEntry
	apiKeyName, apiKeyValue                                  *shortcutEntry
	apiKeyPlacement                                          *widget.Select
	oauth2Grant                                              *widget.Select
	oauth2TokenURL, oauth2AuthURL                            *shortcutEntry
	oauth2ClientID, oauth2ClientSecret, oauth2Scope          *shortcutEntry
	oauth2RedirectURI, oauth2Audience                        *shortcutEntry
	oauth2UsePKCE                                            *widget.Check
	oauth2ClearTokens                                        *widget.Button
	inheritLabel                                             *widget.Label
	nonePanel, inheritPanel                                  *fyne.Container
	basicPanel, bearerPanel, apiKeyPanel, oauth2Panel        *widget.Form
	wssePanel, oauth1Panel, awsPanel, digestPanel, ntlmPanel *widget.Form
	stack                                                    *fyne.Container
	root                                                     fyne.CanvasObject
}

// newAuthEditor builds every widget once; the host embeds e.root. The
// dropdowns' initial selections fire their callbacks during construction, so
// the whole build runs under the loading guard — otherwise a bound target
// (the settings dialogs bind one before load) would be reset to the defaults.
func newAuthEditor(m *MainUI, opts authEditorOpts) *authEditor {
	e := &authEditor{m: m, opts: opts, loading: true}
	defer func() { e.loading = false }()

	labels := authTypeLabels
	if !opts.allowInherit {
		labels = make([]string, 0, len(authTypeLabels)-1)
		for _, l := range authTypeLabels {
			if l != authInheritLabelText {
				labels = append(labels, l)
			}
		}
	}
	e.typeSel = widget.NewSelect(labels, func(label string) {
		if e.suppressed() {
			e.refreshVisibility()
			return
		}
		a := e.opts.target()
		if a == nil {
			return
		}
		a.Type = authTypeByLabel[label]
		e.refreshVisibility()
		e.refreshInheritLabel()
		e.changed()
	})
	e.typeSel.SetSelected(e.defaultLabel())

	e.basicUsername = e.entry("username", func(a *model.Auth, s string) { ensureBasic(a).Username = s })
	e.basicPassword = e.entry("password", func(a *model.Auth, s string) { ensureBasic(a).Password = s })
	e.basicPassword.Password = true

	e.bearerToken = e.entry("token (e.g. {{TOKEN}})", func(a *model.Auth, s string) { ensureBearer(a).Token = s })
	// A bearer token is as sensitive as a Basic password — mask it (#44).
	e.bearerToken.Password = true

	e.digestUsername = e.entry("username", func(a *model.Auth, s string) { ensureDigest(a).Username = s })
	e.digestPassword = e.entry("password", func(a *model.Auth, s string) { ensureDigest(a).Password = s })
	e.digestPassword.Password = true

	e.ntlmUsername = e.entry("username", func(a *model.Auth, s string) { ensureNTLM(a).Username = s })
	e.ntlmPassword = e.entry("password", func(a *model.Auth, s string) { ensureNTLM(a).Password = s })
	e.ntlmPassword.Password = true
	e.ntlmDomain = e.entry("domain (optional)", func(a *model.Auth, s string) { ensureNTLM(a).Domain = s })
	e.ntlmWorkstation = e.entry("workstation (optional)", func(a *model.Auth, s string) { ensureNTLM(a).Workstation = s })

	e.wsseUsername = e.entry("username", func(a *model.Auth, s string) { ensureWSSE(a).Username = s })
	e.wssePassword = e.entry("password", func(a *model.Auth, s string) { ensureWSSE(a).Password = s })
	e.wssePassword.Password = true

	e.oauth1ConsumerKey = e.entry("consumer key", func(a *model.Auth, s string) { ensureOAuth1(a).ConsumerKey = s })
	e.oauth1ConsumerSecret = e.entry("consumer secret", func(a *model.Auth, s string) { ensureOAuth1(a).ConsumerSecret = s })
	e.oauth1ConsumerSecret.Password = true
	e.oauth1Token = e.entry("token (optional)", func(a *model.Auth, s string) { ensureOAuth1(a).Token = s })
	e.oauth1TokenSecret = e.entry("token secret (optional)", func(a *model.Auth, s string) { ensureOAuth1(a).TokenSecret = s })
	e.oauth1TokenSecret.Password = true

	e.awsAccessKey = e.entry("access key id", func(a *model.Auth, s string) { ensureAWSV4(a).AccessKeyID = s })
	e.awsSecretKey = e.entry("secret access key", func(a *model.Auth, s string) { ensureAWSV4(a).SecretAccessKey = s })
	e.awsSecretKey.Password = true
	e.awsRegion = e.entry("region (default us-east-1)", func(a *model.Auth, s string) { ensureAWSV4(a).Region = s })
	e.awsService = e.entry("service (e.g. s3, execute-api)", func(a *model.Auth, s string) { ensureAWSV4(a).Service = s })
	e.awsSessionToken = e.entry("session token (optional, STS)", func(a *model.Auth, s string) { ensureAWSV4(a).SessionToken = s })
	e.awsSessionToken.Password = true

	e.apiKeyName = e.entry("key name (e.g. X-API-Key)", func(a *model.Auth, s string) { ensureAPIKey(a).Name = s })
	e.apiKeyValue = e.entry("key value", func(a *model.Auth, s string) { ensureAPIKey(a).Value = s })
	// The key name is a header/query name (not secret); the value is the
	// credential, so mask it like the other secret fields (#44).
	e.apiKeyValue.Password = true
	e.apiKeyPlacement = widget.NewSelect(apiKeyPlacementLabels, func(label string) {
		e.write(func(a *model.Auth) {
			switch label {
			case "Query":
				ensureAPIKey(a).Placement = model.APIKeyQuery
			default:
				ensureAPIKey(a).Placement = model.APIKeyHeader
			}
		})
	})
	e.apiKeyPlacement.SetSelected("Header")

	e.oauth2Grant = widget.NewSelect(oauth2GrantLabels, func(label string) {
		e.write(func(a *model.Auth) { ensureOAuth2(a).Grant = oauth2GrantByLabel[label] })
	})
	e.oauth2Grant.SetSelected("Client Credentials")
	e.oauth2TokenURL = e.entry("token URL", func(a *model.Auth, s string) { ensureOAuth2(a).TokenURL = s })
	e.oauth2AuthURL = e.entry("authorize URL (authorization_code only)", func(a *model.Auth, s string) { ensureOAuth2(a).AuthURL = s })
	e.oauth2ClientID = e.entry("client id", func(a *model.Auth, s string) { ensureOAuth2(a).ClientID = s })
	e.oauth2ClientSecret = e.entry("client secret", func(a *model.Auth, s string) { ensureOAuth2(a).ClientSecret = s })
	e.oauth2ClientSecret.Password = true
	e.oauth2Scope = e.entry("scope (space-separated)", func(a *model.Auth, s string) { ensureOAuth2(a).Scope = s })
	e.oauth2RedirectURI = e.entry("redirect URI (authorization_code only)", func(a *model.Auth, s string) { ensureOAuth2(a).RedirectURI = s })
	e.oauth2Audience = e.entry("audience (optional)", func(a *model.Auth, s string) { ensureOAuth2(a).Audience = s })
	e.oauth2UsePKCE = widget.NewCheck("Use PKCE (authorization_code)", func(b bool) {
		e.write(func(a *model.Auth) { ensureOAuth2(a).UsePKCE = b })
	})
	e.oauth2ClearTokens = widget.NewButton("Clear cached tokens", func() {
		// Scope the clear to one collection's namespace (the CacheKey prefix
		// used on Send) so other collections' tokens survive.
		ns := ""
		if e.opts.tokenNamespace != nil {
			ns = e.opts.tokenNamespace()
		}
		m.sess.TokenCache().ClearNamespace(ns)
		m.Status.SetText("Cleared cached OAuth2 tokens for this collection")
	})

	e.inheritLabel = widget.NewLabel("")
	e.inheritLabel.Wrapping = fyne.TextWrapWord

	e.nonePanel = container.NewVBox(widget.NewLabel("No authentication will be applied."))
	e.inheritPanel = container.NewVBox(e.inheritLabel)
	e.basicPanel = widget.NewForm(
		widget.NewFormItem("Username", e.basicUsername),
		widget.NewFormItem("Password", e.basicPassword),
	)
	e.bearerPanel = widget.NewForm(
		widget.NewFormItem("Token", e.bearerToken),
	)
	e.digestPanel = widget.NewForm(
		widget.NewFormItem("Username", e.digestUsername),
		widget.NewFormItem("Password", e.digestPassword),
	)
	e.ntlmPanel = widget.NewForm(
		widget.NewFormItem("Username", e.ntlmUsername),
		widget.NewFormItem("Password", e.ntlmPassword),
		widget.NewFormItem("Domain", e.ntlmDomain),
		widget.NewFormItem("Workstation", e.ntlmWorkstation),
	)
	e.apiKeyPanel = widget.NewForm(
		widget.NewFormItem("Name", e.apiKeyName),
		widget.NewFormItem("Value", e.apiKeyValue),
		widget.NewFormItem("Placement", e.apiKeyPlacement),
	)
	e.oauth2Panel = widget.NewForm(
		widget.NewFormItem("Grant", e.oauth2Grant),
		widget.NewFormItem("Token URL", e.oauth2TokenURL),
		widget.NewFormItem("Authorize URL", e.oauth2AuthURL),
		widget.NewFormItem("Client ID", e.oauth2ClientID),
		widget.NewFormItem("Client Secret", e.oauth2ClientSecret),
		widget.NewFormItem("Scope", e.oauth2Scope),
		widget.NewFormItem("Redirect URI", e.oauth2RedirectURI),
		widget.NewFormItem("Audience", e.oauth2Audience),
		widget.NewFormItem("PKCE", e.oauth2UsePKCE),
		widget.NewFormItem("Cache", e.oauth2ClearTokens),
	)
	e.wssePanel = widget.NewForm(
		widget.NewFormItem("Username", e.wsseUsername),
		widget.NewFormItem("Password", e.wssePassword),
	)
	e.oauth1Panel = widget.NewForm(
		widget.NewFormItem("Consumer Key", e.oauth1ConsumerKey),
		widget.NewFormItem("Consumer Secret", e.oauth1ConsumerSecret),
		widget.NewFormItem("Token", e.oauth1Token),
		widget.NewFormItem("Token Secret", e.oauth1TokenSecret),
	)
	e.awsPanel = widget.NewForm(
		widget.NewFormItem("Access Key ID", e.awsAccessKey),
		widget.NewFormItem("Secret Access Key", e.awsSecretKey),
		widget.NewFormItem("Region", e.awsRegion),
		widget.NewFormItem("Service", e.awsService),
		widget.NewFormItem("Session Token", e.awsSessionToken),
	)

	e.stack = container.NewStack(e.panels()...)
	top := container.NewBorder(nil, nil, widget.NewLabel("Type:"), nil, e.typeSel)
	e.root = container.NewBorder(top, nil, nil, nil, container.NewVScroll(e.stack))
	e.refreshVisibility()
	e.refreshInheritLabel()
	return e
}

// defaultLabel is the Type shown for a zero / Inherit Auth: Inherit where the
// host allows it, None for the collection root.
func (e *authEditor) defaultLabel() string {
	if e.opts.allowInherit {
		return authInheritLabelText
	}
	return "None"
}

// panels lists every stacked panel in a fixed order (refreshVisibility hides
// them all before showing the active one).
func (e *authEditor) panels() []fyne.CanvasObject {
	return []fyne.CanvasObject{
		e.nonePanel, e.inheritPanel,
		e.basicPanel, e.bearerPanel,
		e.apiKeyPanel, e.oauth2Panel, e.wssePanel, e.oauth1Panel, e.awsPanel,
		e.digestPanel, e.ntlmPanel,
	}
}

// suppressed reports whether write-back is currently disabled: the editor is
// loading, or the host says it is busy (the request tab's m.loading).
func (e *authEditor) suppressed() bool {
	return e.loading || (e.opts.busy != nil && e.opts.busy())
}

func (e *authEditor) changed() {
	if e.opts.onChange != nil {
		e.opts.onChange()
	}
}

// write applies fn to the target Auth unless suppressed or unbound, then
// signals the change. Every widget callback funnels through here.
func (e *authEditor) write(fn func(*model.Auth)) {
	if e.suppressed() {
		return
	}
	a := e.opts.target()
	if a == nil {
		return
	}
	fn(a)
	e.changed()
}

// entry returns a shortcutEntry whose OnChanged writes through to the target
// via write (honouring the suppression guards).
func (e *authEditor) entry(placeholder string, set func(*model.Auth, string)) *shortcutEntry {
	en := e.m.newShortcutEntry()
	en.SetPlaceHolder(placeholder)
	en.OnChanged = func(s string) {
		e.write(func(a *model.Auth) { set(a, s) })
	}
	return en
}

// selectedType is the model type behind the dropdown's current label.
func (e *authEditor) selectedType() model.AuthType {
	return authTypeByLabel[e.typeSel.Selected]
}

// refreshVisibility hides every panel in the stack and then shows only the
// one matching the selected Type. Called after the user changes the dropdown
// and at the end of load.
func (e *authEditor) refreshVisibility() {
	if e.stack == nil {
		return
	}
	for _, p := range e.panels() {
		p.Hide()
	}
	var active fyne.CanvasObject
	switch e.selectedType() {
	case model.AuthNone:
		active = e.nonePanel
	case model.AuthInherit:
		active = e.inheritPanel
	case model.AuthBasic:
		active = e.basicPanel
	case model.AuthDigest:
		active = e.digestPanel
	case model.AuthNTLM:
		active = e.ntlmPanel
	case model.AuthBearer:
		active = e.bearerPanel
	case model.AuthAPIKey:
		active = e.apiKeyPanel
	case model.AuthOAuth1:
		active = e.oauth1Panel
	case model.AuthOAuth2:
		active = e.oauth2Panel
	case model.AuthWSSE:
		active = e.wssePanel
	case model.AuthAWSV4:
		active = e.awsPanel
	default:
		active = e.inheritPanel
	}
	active.Show()
	e.stack.Refresh()
}

// refreshInheritLabel re-computes the Inherit panel's preview via
// opts.inheritText so the user can see at a glance whether "Inherit" would
// actually do anything useful.
func (e *authEditor) refreshInheritLabel() {
	if e.inheritLabel == nil {
		return
	}
	if e.opts.inheritText == nil {
		e.inheritLabel.SetText("Inheriting — the nearest auth configured on an enclosing folder or the collection applies.")
		return
	}
	e.inheritLabel.SetText(e.opts.inheritText())
}

// inheritPreview formats the Inherit panel's line for a resolved ancestor
// auth (the result of Session.InheritedAuth).
func inheritPreview(eff model.Auth) string {
	if eff.Type == model.AuthNone || eff.Type == "" {
		return "Inheriting — no auth is configured on any ancestor. Effective: None."
	}
	label := authLabelByType[eff.Type]
	if label == "" {
		label = string(eff.Type)
	}
	return fmt.Sprintf("Inheriting — effective auth: %s.", label)
}

// load pushes a into every widget without firing write-back: the Type
// dropdown, the matching sub-struct's fields, and blanks for every other
// sub-form so stale data from a previous target never shows.
func (e *authEditor) load(a model.Auth) {
	e.loading = true
	defer func() { e.loading = false }()

	t := a.Type
	if t == "" {
		t = model.AuthInherit
	}
	if label, ok := authLabelByType[t]; ok && (t != model.AuthInherit || e.opts.allowInherit) {
		e.typeSel.SetSelected(label)
	} else {
		e.typeSel.SetSelected(e.defaultLabel())
	}

	setText := func(en *shortcutEntry, s string) { en.SetText(s) }
	if b := a.Basic; b != nil {
		setText(e.basicUsername, b.Username)
		setText(e.basicPassword, b.Password)
	} else {
		setText(e.basicUsername, "")
		setText(e.basicPassword, "")
	}
	if d := a.Digest; d != nil {
		setText(e.digestUsername, d.Username)
		setText(e.digestPassword, d.Password)
	} else {
		setText(e.digestUsername, "")
		setText(e.digestPassword, "")
	}
	if n := a.NTLM; n != nil {
		setText(e.ntlmUsername, n.Username)
		setText(e.ntlmPassword, n.Password)
		setText(e.ntlmDomain, n.Domain)
		setText(e.ntlmWorkstation, n.Workstation)
	} else {
		setText(e.ntlmUsername, "")
		setText(e.ntlmPassword, "")
		setText(e.ntlmDomain, "")
		setText(e.ntlmWorkstation, "")
	}
	if br := a.Bearer; br != nil {
		setText(e.bearerToken, br.Token)
	} else {
		setText(e.bearerToken, "")
	}
	if w := a.WSSE; w != nil {
		setText(e.wsseUsername, w.Username)
		setText(e.wssePassword, w.Password)
	} else {
		setText(e.wsseUsername, "")
		setText(e.wssePassword, "")
	}
	if o := a.OAuth1; o != nil {
		setText(e.oauth1ConsumerKey, o.ConsumerKey)
		setText(e.oauth1ConsumerSecret, o.ConsumerSecret)
		setText(e.oauth1Token, o.Token)
		setText(e.oauth1TokenSecret, o.TokenSecret)
	} else {
		setText(e.oauth1ConsumerKey, "")
		setText(e.oauth1ConsumerSecret, "")
		setText(e.oauth1Token, "")
		setText(e.oauth1TokenSecret, "")
	}
	if v := a.AWSV4; v != nil {
		setText(e.awsAccessKey, v.AccessKeyID)
		setText(e.awsSecretKey, v.SecretAccessKey)
		setText(e.awsRegion, v.Region)
		setText(e.awsService, v.Service)
		setText(e.awsSessionToken, v.SessionToken)
	} else {
		setText(e.awsAccessKey, "")
		setText(e.awsSecretKey, "")
		setText(e.awsRegion, "")
		setText(e.awsService, "")
		setText(e.awsSessionToken, "")
	}
	if k := a.APIKey; k != nil {
		setText(e.apiKeyName, k.Name)
		setText(e.apiKeyValue, k.Value)
		if k.Placement == model.APIKeyQuery {
			e.apiKeyPlacement.SetSelected("Query")
		} else {
			e.apiKeyPlacement.SetSelected("Header")
		}
	} else {
		setText(e.apiKeyName, "")
		setText(e.apiKeyValue, "")
		e.apiKeyPlacement.SetSelected("Header")
	}
	if o := a.OAuth2; o != nil {
		if label, ok := oauth2LabelByGrant[o.Grant]; ok {
			e.oauth2Grant.SetSelected(label)
		} else {
			e.oauth2Grant.SetSelected("Client Credentials")
		}
		setText(e.oauth2TokenURL, o.TokenURL)
		setText(e.oauth2AuthURL, o.AuthURL)
		setText(e.oauth2ClientID, o.ClientID)
		setText(e.oauth2ClientSecret, o.ClientSecret)
		setText(e.oauth2Scope, o.Scope)
		setText(e.oauth2RedirectURI, o.RedirectURI)
		setText(e.oauth2Audience, o.Audience)
		e.oauth2UsePKCE.SetChecked(o.UsePKCE)
	} else {
		e.oauth2Grant.SetSelected("Client Credentials")
		setText(e.oauth2TokenURL, "")
		setText(e.oauth2AuthURL, "")
		setText(e.oauth2ClientID, "")
		setText(e.oauth2ClientSecret, "")
		setText(e.oauth2Scope, "")
		setText(e.oauth2RedirectURI, "")
		setText(e.oauth2Audience, "")
		e.oauth2UsePKCE.SetChecked(false)
	}
	e.refreshVisibility()
	e.refreshInheritLabel()
}

// Lazy allocators for the per-scheme sub-structs: a widget's write-back
// allocates the struct its Type needs the first time the user types into it.

func ensureBasic(a *model.Auth) *model.BasicAuth {
	if a.Basic == nil {
		a.Basic = &model.BasicAuth{}
	}
	return a.Basic
}

func ensureBearer(a *model.Auth) *model.BearerAuth {
	if a.Bearer == nil {
		a.Bearer = &model.BearerAuth{}
	}
	return a.Bearer
}

func ensureDigest(a *model.Auth) *model.DigestAuth {
	if a.Digest == nil {
		a.Digest = &model.DigestAuth{}
	}
	return a.Digest
}

func ensureNTLM(a *model.Auth) *model.NTLMAuth {
	if a.NTLM == nil {
		a.NTLM = &model.NTLMAuth{}
	}
	return a.NTLM
}

func ensureWSSE(a *model.Auth) *model.WSSEAuth {
	if a.WSSE == nil {
		a.WSSE = &model.WSSEAuth{}
	}
	return a.WSSE
}

func ensureOAuth1(a *model.Auth) *model.OAuth1Auth {
	if a.OAuth1 == nil {
		a.OAuth1 = &model.OAuth1Auth{}
	}
	return a.OAuth1
}

func ensureAWSV4(a *model.Auth) *model.AWSV4Auth {
	if a.AWSV4 == nil {
		a.AWSV4 = &model.AWSV4Auth{}
	}
	return a.AWSV4
}

func ensureAPIKey(a *model.Auth) *model.APIKeyAuth {
	if a.APIKey == nil {
		a.APIKey = &model.APIKeyAuth{Placement: model.APIKeyHeader}
	}
	return a.APIKey
}

func ensureOAuth2(a *model.Auth) *model.OAuth2Auth {
	if a.OAuth2 == nil {
		a.OAuth2 = &model.OAuth2Auth{Grant: model.OAuth2ClientCredentials}
	}
	return a.OAuth2
}
