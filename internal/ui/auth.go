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
var authTypeLabels = []string{"None", "Inherit from parent", "Basic Auth", "Bearer Token", "API Key", "OAuth 1.0a", "OAuth 2.0", "WS-Security"}

var authTypeByLabel = map[string]model.AuthType{
	"None":                model.AuthNone,
	"Inherit from parent": model.AuthInherit,
	"Basic Auth":          model.AuthBasic,
	"Bearer Token":        model.AuthBearer,
	"API Key":             model.AuthAPIKey,
	"OAuth 1.0a":          model.AuthOAuth1,
	"OAuth 2.0":           model.AuthOAuth2,
	"WS-Security":         model.AuthWSSE,
}

var authLabelByType = map[model.AuthType]string{
	model.AuthNone:    "None",
	model.AuthInherit: "Inherit from parent",
	model.AuthBasic:   "Basic Auth",
	model.AuthBearer:  "Bearer Token",
	model.AuthAPIKey:  "API Key",
	model.AuthOAuth1:  "OAuth 1.0a",
	model.AuthOAuth2:  "OAuth 2.0",
	model.AuthWSSE:    "WS-Security",
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

// buildAuthTab assembles the per-request Auth tab: a Type dropdown plus a
// stacked column of type-specific form panels. Only the panel matching the
// selected Type is visible. The Inherit panel surfaces what would actually
// be applied by walking the ancestor chain via session.EffectiveAuth.
func (m *MainUI) buildAuthTab() fyne.CanvasObject {
	m.authType = widget.NewSelect(authTypeLabels, func(label string) {
		if m.loading {
			m.refreshAuthVisibility()
			return
		}
		if m.currentRequest == nil {
			return
		}
		m.currentRequest.Auth.Type = authTypeByLabel[label]
		m.refreshAuthVisibility()
		m.refreshAuthInheritLabel()
	})
	m.authType.SetSelected("Inherit from parent")

	m.authBasicUsername = m.newAuthEntry("username", func(s string) {
		m.ensureBasic().Username = s
	})
	m.authBasicPassword = m.newAuthEntry("password", func(s string) {
		m.ensureBasic().Password = s
	})
	m.authBasicPassword.Password = true

	m.authBearerToken = m.newAuthEntry("token (e.g. {{TOKEN}})", func(s string) {
		m.ensureBearer().Token = s
	})
	// A bearer token is as sensitive as a Basic password — mask it (#44).
	m.authBearerToken.Password = true

	m.authWSSEUsername = m.newAuthEntry("username", func(s string) {
		m.ensureWSSE().Username = s
	})
	m.authWSSEPassword = m.newAuthEntry("password", func(s string) {
		m.ensureWSSE().Password = s
	})
	m.authWSSEPassword.Password = true

	m.authOAuth1ConsumerKey = m.newAuthEntry("consumer key", func(s string) {
		m.ensureOAuth1().ConsumerKey = s
	})
	m.authOAuth1ConsumerSecret = m.newAuthEntry("consumer secret", func(s string) {
		m.ensureOAuth1().ConsumerSecret = s
	})
	m.authOAuth1ConsumerSecret.Password = true
	m.authOAuth1Token = m.newAuthEntry("token (optional)", func(s string) {
		m.ensureOAuth1().Token = s
	})
	m.authOAuth1TokenSecret = m.newAuthEntry("token secret (optional)", func(s string) {
		m.ensureOAuth1().TokenSecret = s
	})
	m.authOAuth1TokenSecret.Password = true

	m.authAPIKeyName = m.newAuthEntry("key name (e.g. X-API-Key)", func(s string) {
		m.ensureAPIKey().Name = s
	})
	m.authAPIKeyValue = m.newAuthEntry("key value", func(s string) {
		m.ensureAPIKey().Value = s
	})
	// The key name is a header/query name (not secret); the value is the
	// credential, so mask it like the other secret fields (#44).
	m.authAPIKeyValue.Password = true
	m.authAPIKeyPlacement = widget.NewSelect(apiKeyPlacementLabels, func(label string) {
		if m.loading || m.currentRequest == nil {
			return
		}
		switch label {
		case "Query":
			m.ensureAPIKey().Placement = model.APIKeyQuery
		default:
			m.ensureAPIKey().Placement = model.APIKeyHeader
		}
	})
	m.authAPIKeyPlacement.SetSelected("Header")

	m.authOAuth2Grant = widget.NewSelect(oauth2GrantLabels, func(label string) {
		if m.loading || m.currentRequest == nil {
			return
		}
		m.ensureOAuth2().Grant = oauth2GrantByLabel[label]
	})
	m.authOAuth2Grant.SetSelected("Client Credentials")
	m.authOAuth2TokenURL = m.newAuthEntry("token URL", func(s string) { m.ensureOAuth2().TokenURL = s })
	m.authOAuth2AuthURL = m.newAuthEntry("authorize URL (authorization_code only)", func(s string) { m.ensureOAuth2().AuthURL = s })
	m.authOAuth2ClientID = m.newAuthEntry("client id", func(s string) { m.ensureOAuth2().ClientID = s })
	m.authOAuth2ClientSecret = m.newAuthEntry("client secret", func(s string) { m.ensureOAuth2().ClientSecret = s })
	m.authOAuth2ClientSecret.Password = true
	m.authOAuth2Scope = m.newAuthEntry("scope (space-separated)", func(s string) { m.ensureOAuth2().Scope = s })
	m.authOAuth2RedirectURI = m.newAuthEntry("redirect URI (authorization_code only)", func(s string) { m.ensureOAuth2().RedirectURI = s })
	m.authOAuth2Audience = m.newAuthEntry("audience (optional)", func(s string) { m.ensureOAuth2().Audience = s })
	m.authOAuth2UsePKCE = widget.NewCheck("Use PKCE (authorization_code)", func(b bool) {
		if m.loading || m.currentRequest == nil {
			return
		}
		m.ensureOAuth2().UsePKCE = b
	})
	m.authOAuth2ClearTokens = widget.NewButton("Clear cached tokens", func() {
		// Scope the clear to the active collection's namespace (the CacheKey
		// prefix used on Send) so other collections' tokens survive.
		m.sess.TokenCache().ClearNamespace(m.sess.ActiveCollectionDir())
		m.Status.SetText("Cleared cached OAuth2 tokens for this collection")
	})

	m.authInheritLabel = widget.NewLabel("")
	m.authInheritLabel.Wrapping = fyne.TextWrapWord

	m.authNonePanel = container.NewVBox(widget.NewLabel("No authentication will be applied."))
	m.authInheritPanel = container.NewVBox(m.authInheritLabel)
	m.authBasicPanel = widget.NewForm(
		widget.NewFormItem("Username", m.authBasicUsername),
		widget.NewFormItem("Password", m.authBasicPassword),
	)
	m.authBearerPanel = widget.NewForm(
		widget.NewFormItem("Token", m.authBearerToken),
	)
	m.authAPIKeyPanel = widget.NewForm(
		widget.NewFormItem("Name", m.authAPIKeyName),
		widget.NewFormItem("Value", m.authAPIKeyValue),
		widget.NewFormItem("Placement", m.authAPIKeyPlacement),
	)
	m.authOAuth2Panel = widget.NewForm(
		widget.NewFormItem("Grant", m.authOAuth2Grant),
		widget.NewFormItem("Token URL", m.authOAuth2TokenURL),
		widget.NewFormItem("Authorize URL", m.authOAuth2AuthURL),
		widget.NewFormItem("Client ID", m.authOAuth2ClientID),
		widget.NewFormItem("Client Secret", m.authOAuth2ClientSecret),
		widget.NewFormItem("Scope", m.authOAuth2Scope),
		widget.NewFormItem("Redirect URI", m.authOAuth2RedirectURI),
		widget.NewFormItem("Audience", m.authOAuth2Audience),
		widget.NewFormItem("PKCE", m.authOAuth2UsePKCE),
		widget.NewFormItem("Cache", m.authOAuth2ClearTokens),
	)

	m.authWSSEPanel = widget.NewForm(
		widget.NewFormItem("Username", m.authWSSEUsername),
		widget.NewFormItem("Password", m.authWSSEPassword),
	)
	m.authOAuth1Panel = widget.NewForm(
		widget.NewFormItem("Consumer Key", m.authOAuth1ConsumerKey),
		widget.NewFormItem("Consumer Secret", m.authOAuth1ConsumerSecret),
		widget.NewFormItem("Token", m.authOAuth1Token),
		widget.NewFormItem("Token Secret", m.authOAuth1TokenSecret),
	)

	m.authFormsStack = container.NewStack(
		m.authNonePanel, m.authInheritPanel,
		m.authBasicPanel, m.authBearerPanel,
		m.authAPIKeyPanel, m.authOAuth2Panel, m.authWSSEPanel, m.authOAuth1Panel,
	)

	top := container.NewBorder(nil, nil, widget.NewLabel("Type:"), nil, m.authType)
	return container.NewBorder(top, nil, nil, nil, container.NewVScroll(m.authFormsStack))
}

// newAuthEntry returns a new widget.Entry whose write-back closure honors
// m.loading (so loadRequest can populate without round-tripping back to
// the model) and skips writes when there's no current request loaded.
func (m *MainUI) newAuthEntry(placeholder string, write func(string)) *widget.Entry {
	e := widget.NewEntry()
	e.SetPlaceHolder(placeholder)
	e.OnChanged = func(s string) {
		if m.loading || m.currentRequest == nil {
			return
		}
		write(s)
	}
	return e
}

func (m *MainUI) ensureBasic() *model.BasicAuth {
	if m.currentRequest.Auth.Basic == nil {
		m.currentRequest.Auth.Basic = &model.BasicAuth{}
	}
	return m.currentRequest.Auth.Basic
}

func (m *MainUI) ensureBearer() *model.BearerAuth {
	if m.currentRequest.Auth.Bearer == nil {
		m.currentRequest.Auth.Bearer = &model.BearerAuth{}
	}
	return m.currentRequest.Auth.Bearer
}

func (m *MainUI) ensureWSSE() *model.WSSEAuth {
	if m.currentRequest.Auth.WSSE == nil {
		m.currentRequest.Auth.WSSE = &model.WSSEAuth{}
	}
	return m.currentRequest.Auth.WSSE
}

func (m *MainUI) ensureOAuth1() *model.OAuth1Auth {
	if m.currentRequest.Auth.OAuth1 == nil {
		m.currentRequest.Auth.OAuth1 = &model.OAuth1Auth{}
	}
	return m.currentRequest.Auth.OAuth1
}

func (m *MainUI) ensureAPIKey() *model.APIKeyAuth {
	if m.currentRequest.Auth.APIKey == nil {
		m.currentRequest.Auth.APIKey = &model.APIKeyAuth{Placement: model.APIKeyHeader}
	}
	return m.currentRequest.Auth.APIKey
}

func (m *MainUI) ensureOAuth2() *model.OAuth2Auth {
	if m.currentRequest.Auth.OAuth2 == nil {
		m.currentRequest.Auth.OAuth2 = &model.OAuth2Auth{Grant: model.OAuth2ClientCredentials}
	}
	return m.currentRequest.Auth.OAuth2
}

// refreshAuthVisibility hides every panel in the auth forms stack and
// then shows only the one matching the selected Type. Called both after
// the user changes the dropdown and during loadRequest.
func (m *MainUI) refreshAuthVisibility() {
	if m.authFormsStack == nil {
		return
	}
	panels := []fyne.CanvasObject{
		m.authNonePanel, m.authInheritPanel,
		m.authBasicPanel, m.authBearerPanel,
		m.authAPIKeyPanel, m.authOAuth2Panel, m.authWSSEPanel, m.authOAuth1Panel,
	}
	for _, p := range panels {
		p.Hide()
	}
	var active fyne.CanvasObject
	switch authTypeByLabel[m.authType.Selected] {
	case model.AuthNone:
		active = m.authNonePanel
	case model.AuthInherit:
		active = m.authInheritPanel
	case model.AuthBasic:
		active = m.authBasicPanel
	case model.AuthBearer:
		active = m.authBearerPanel
	case model.AuthAPIKey:
		active = m.authAPIKeyPanel
	case model.AuthOAuth1:
		active = m.authOAuth1Panel
	case model.AuthOAuth2:
		active = m.authOAuth2Panel
	case model.AuthWSSE:
		active = m.authWSSEPanel
	default:
		active = m.authInheritPanel
	}
	active.Show()
	m.authFormsStack.Refresh()
}

// refreshAuthInheritLabel re-computes and displays what the Inherit panel
// would resolve to via session.EffectiveAuth. Called after type changes
// and after loading a request so the user can see at a glance whether
// "Inherit" would actually do anything useful.
func (m *MainUI) refreshAuthInheritLabel() {
	if m.authInheritLabel == nil {
		return
	}
	if m.currentRequestID == "" {
		m.authInheritLabel.SetText("Inheriting — no resolved auth available (no request selected).")
		return
	}
	eff := m.sess.EffectiveAuth(m.currentRequestID)
	if eff.Type == model.AuthNone || eff.Type == "" {
		m.authInheritLabel.SetText("Inheriting — no auth is configured on any ancestor. Effective: None.")
		return
	}
	label := authLabelByType[eff.Type]
	if label == "" {
		label = string(eff.Type)
	}
	m.authInheritLabel.SetText(fmt.Sprintf("Inheriting — effective auth: %s.", label))
}

// loadAuthTab populates every auth widget from req.Auth without firing
// write-back callbacks. Called from loadRequest under the m.loading guard.
func (m *MainUI) loadAuthTab(req *model.Request) {
	if m.authType == nil {
		return
	}
	if req == nil {
		m.authType.SetSelected("Inherit from parent")
		m.authBasicUsername.SetText("")
		m.authBasicPassword.SetText("")
		m.authBearerToken.SetText("")
		m.authWSSEUsername.SetText("")
		m.authWSSEPassword.SetText("")
		m.authOAuth1ConsumerKey.SetText("")
		m.authOAuth1ConsumerSecret.SetText("")
		m.authOAuth1Token.SetText("")
		m.authOAuth1TokenSecret.SetText("")
		m.authAPIKeyName.SetText("")
		m.authAPIKeyValue.SetText("")
		m.authAPIKeyPlacement.SetSelected("Header")
		m.authOAuth2Grant.SetSelected("Client Credentials")
		m.authOAuth2TokenURL.SetText("")
		m.authOAuth2AuthURL.SetText("")
		m.authOAuth2ClientID.SetText("")
		m.authOAuth2ClientSecret.SetText("")
		m.authOAuth2Scope.SetText("")
		m.authOAuth2RedirectURI.SetText("")
		m.authOAuth2Audience.SetText("")
		m.authOAuth2UsePKCE.SetChecked(false)
		m.refreshAuthVisibility()
		m.refreshAuthInheritLabel()
		return
	}
	t := req.Auth.Type
	if t == "" {
		t = model.AuthInherit
	}
	if label, ok := authLabelByType[t]; ok {
		m.authType.SetSelected(label)
	} else {
		m.authType.SetSelected("Inherit from parent")
	}
	if b := req.Auth.Basic; b != nil {
		m.authBasicUsername.SetText(b.Username)
		m.authBasicPassword.SetText(b.Password)
	} else {
		m.authBasicUsername.SetText("")
		m.authBasicPassword.SetText("")
	}
	if br := req.Auth.Bearer; br != nil {
		m.authBearerToken.SetText(br.Token)
	} else {
		m.authBearerToken.SetText("")
	}
	if w := req.Auth.WSSE; w != nil {
		m.authWSSEUsername.SetText(w.Username)
		m.authWSSEPassword.SetText(w.Password)
	} else {
		m.authWSSEUsername.SetText("")
		m.authWSSEPassword.SetText("")
	}
	if o := req.Auth.OAuth1; o != nil {
		m.authOAuth1ConsumerKey.SetText(o.ConsumerKey)
		m.authOAuth1ConsumerSecret.SetText(o.ConsumerSecret)
		m.authOAuth1Token.SetText(o.Token)
		m.authOAuth1TokenSecret.SetText(o.TokenSecret)
	} else {
		m.authOAuth1ConsumerKey.SetText("")
		m.authOAuth1ConsumerSecret.SetText("")
		m.authOAuth1Token.SetText("")
		m.authOAuth1TokenSecret.SetText("")
	}
	if k := req.Auth.APIKey; k != nil {
		m.authAPIKeyName.SetText(k.Name)
		m.authAPIKeyValue.SetText(k.Value)
		if k.Placement == model.APIKeyQuery {
			m.authAPIKeyPlacement.SetSelected("Query")
		} else {
			m.authAPIKeyPlacement.SetSelected("Header")
		}
	} else {
		m.authAPIKeyName.SetText("")
		m.authAPIKeyValue.SetText("")
		m.authAPIKeyPlacement.SetSelected("Header")
	}
	if o := req.Auth.OAuth2; o != nil {
		if label, ok := oauth2LabelByGrant[o.Grant]; ok {
			m.authOAuth2Grant.SetSelected(label)
		} else {
			m.authOAuth2Grant.SetSelected("Client Credentials")
		}
		m.authOAuth2TokenURL.SetText(o.TokenURL)
		m.authOAuth2AuthURL.SetText(o.AuthURL)
		m.authOAuth2ClientID.SetText(o.ClientID)
		m.authOAuth2ClientSecret.SetText(o.ClientSecret)
		m.authOAuth2Scope.SetText(o.Scope)
		m.authOAuth2RedirectURI.SetText(o.RedirectURI)
		m.authOAuth2Audience.SetText(o.Audience)
		m.authOAuth2UsePKCE.SetChecked(o.UsePKCE)
	} else {
		m.authOAuth2Grant.SetSelected("Client Credentials")
		m.authOAuth2TokenURL.SetText("")
		m.authOAuth2AuthURL.SetText("")
		m.authOAuth2ClientID.SetText("")
		m.authOAuth2ClientSecret.SetText("")
		m.authOAuth2Scope.SetText("")
		m.authOAuth2RedirectURI.SetText("")
		m.authOAuth2Audience.SetText("")
		m.authOAuth2UsePKCE.SetChecked(false)
	}
	m.refreshAuthVisibility()
	m.refreshAuthInheritLabel()
}
