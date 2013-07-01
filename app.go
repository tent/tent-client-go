package tent

import (
	"crypto/sha256"
	"encoding/json"
	"net/http"

	"github.com/tent/hawk-go"
)

const (
	PostTypeApp         = "https://tent.io/types/app/v0#"
	PostTypeAppAuth     = "https://tent.io/types/app-auth/v0#"
	PostTypeCredentials = "https://tent.io/types/credentials/v0#"
)

type Credentials struct {
	HawkKey       string `json:"hawk_key"`
	HawkAlgorithm string `json:"hawk_algorithm"`

	Post *Post
}

func NewAppPost(app *App) *Post {
	data, _ := json.Marshal(app)
	return &Post{Type: PostTypeApp, Content: data, Permissions: &PostPermissions{PublicFlag: new(bool)}}
}

type AppTypes struct {
	Read  []string `json:"read,omitempty"`
	Write []string `json:"write,omitempty"`
}

type App struct {
	Name        string   `json:"name"`
	URL         string   `json:"url"`
	Description string   `json:"description,omitempty"`
	Scopes      []string `json:"scopes,omitempty"`

	Types AppTypes `json:"types,omitempty"`

	RedirectURI       string   `json:"redirect_uri"`
	NotificationURL   string   `json:"notification_url,omitempty"`
	NotificationTypes []string `json:"notification_types,omitempty"`

	Post *Post `json:"-"`
}

type AppAuth struct {
	Active bool     `json:"active"`
	Scopes []string `json:"scopes,omitempty"`

	Types AppTypes `json:"types,omitempty"`

	Post *Post `json:"-"`
}

const TokenTypeHawk = "https://tent.io/oauth/hawk-token"

func oauthTokenURL(server *MetaPostServer) string { return server.URLs.OAuthToken }

func (client *Client) RequestAccessToken(code string) (*hawk.Credentials, error) {
	data, _ := json.Marshal(&AccessTokenRequest{TokenType: TokenTypeHawk, Code: code})
	tokenRes := &AccessTokenResponse{}
	header := make(http.Header)
	header.Set("Accept", "application/json")
	header.Set("Content-Type", "application/json")
	_, err := client.requestJSON("POST", oauthTokenURL, header, data, tokenRes)
	if err != nil {
		return nil, err
	}
	return tokenRes.HawkCredentials(client.Credentials.App), err
}

type AccessTokenRequest struct {
	Code      string `json:"code"`
	TokenType string `json:"token_type"`
}

type AccessTokenResponse struct {
	HawkID  string `json:"access_token"`
	HawkKey string `json:"hawk_key"`

	HawkAlgorithm string `json:"hawk_algorithm"`
	TokenType     string `json:"token_type"`
}

func (res *AccessTokenResponse) HawkCredentials(app string) *hawk.Credentials {
	return &hawk.Credentials{Key: res.HawkKey, ID: res.HawkID, App: app, Hash: sha256.New}
}
