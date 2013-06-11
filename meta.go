package tent

import (
	"encoding/json"
	"net/url"
	"strings"
)

const RelMetaPost = "https://tent.io/rels/meta-post"

type MetaPostServer struct {
	Version    string `json:"version"`
	Preference int    `json:"preference"`

	URLs MetaPostServerURLs `json:"urls"`
}

type MetaPostServerURLs struct {
	OAuthAuth      string `json:"oauth_auth"`
	OAuthToken     string `json:"oauth_token"`
	PostsFeed      string `json:"posts_feed"`
	Post           string `json:"post"`
	NewPost        string `json:"new_post"`
	PostAttachment string `json:"post_attachment"`
	Attachment     string `json:"attachment"`
	Batch          string `json:"batch"`
	ServerInfo     string `json:"server_info"`
}

func (urls *MetaPostServerURLs) OAuthURL(appID string, state string) (string, error) {
	u, err := url.Parse(urls.OAuthAuth)
	if err != nil {
		return "", err
	}
	u.RawQuery = url.Values{"client_id": {appID}, "state": {state}}.Encode()
	return u.String(), nil
}

func (urls *MetaPostServerURLs) PostURL(entity, post, version string) (string, error) {
	u := strings.Replace(urls.Post, "{entity}", url.QueryEscape(entity), 1)
	u = strings.Replace(u, "{post}", post, 1)
	if version != "" {
		if strings.Contains(u, "?") {
			uri, err := url.Parse(u)
			if err != nil {
				return "", err
			}
			q := uri.Query()
			q.Add("version", version)
			uri.RawQuery = q.Encode()
			u = uri.String()
		} else {
			u += "?version=" + version
		}
	}
	return u, nil
}

func (urls *MetaPostServerURLs) PostAttachmentURL(entity, post, version, name string) string {
	u := strings.Replace(urls.PostAttachment, "{entity}", url.QueryEscape(entity), 1)
	u = strings.Replace(u, "{post}", post, 1)
	u = strings.Replace(u, "{name}", url.QueryEscape(name), 1)
	return strings.Replace(u, "{version}", version, 1)
}

func (urls *MetaPostServerURLs) AttachmentURL(entity, digest string) string {
	u := strings.Replace(urls.Attachment, "{entity}", url.QueryEscape(entity), 1)
	return strings.Replace(u, "{digest}", digest, 1)
}

type MetaPost struct {
	Entity  string           `json:"entity"`
	Servers []MetaPostServer `json:"servers"`
	Post    *Post            `json:"-"`
}

func GetMetaPost(url string) (*MetaPost, error) {
	post, err := GetPost(url)
	if err != nil {
		return nil, err
	}
	metaPost := &MetaPost{Post: post.Post}
	err = json.Unmarshal(post.Post.Content, metaPost)
	return metaPost, err
}

func getMetaPost(links []string, reqURL *url.URL) (*MetaPost, error) {
	for i, l := range links {
		u, err := url.Parse(l)
		if err != nil {
			return nil, err
		}
		m, err := GetMetaPost(reqURL.ResolveReference(u).String())
		if err != nil && i < len(links)-1 {
			continue
		}
		return m, err
	}
	panic("not reached")
}
