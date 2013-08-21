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

func (urls *MetaPostServerURLs) OAuthURL(appID string, state string) string {
	return appendQuery(urls.OAuthAuth, url.Values{"client_id": {appID}, "state": {state}}.Encode())
}

func (urls *MetaPostServerURLs) PostURL(entity, post, version string) string {
	u := strings.Replace(urls.Post, "{entity}", url.QueryEscape(entity), 1)
	u = strings.Replace(u, "{post}", post, 1)
	if version != "" {
		u = appendQuery(u, "version="+version)
	}
	return u
}

func (urls *MetaPostServerURLs) PostAttachmentURL(entity, post, version, name string) string {
	u := strings.Replace(urls.PostAttachment, "{entity}", url.QueryEscape(entity), 1)
	u = strings.Replace(u, "{post}", post, 1)
	u = strings.Replace(u, "{name}", url.QueryEscape(name), 1)
	if version != "" {
		u = appendQuery(u, "version="+version)
	}
	return u
}

func (urls *MetaPostServerURLs) AttachmentURL(entity, digest string) string {
	u := strings.Replace(urls.Attachment, "{entity}", url.QueryEscape(entity), 1)
	return strings.Replace(u, "{digest}", digest, 1)
}

func appendQuery(url, query string) string {
	if strings.Contains(url, "?") {
		return url + "&" + query
	}
	return url + "?" + query
}

type MetaPost struct {
	Entity  string           `json:"entity"`
	Profile MetaProfile      `json:"profile"`
	Servers []MetaPostServer `json:"servers"`
	Post    *Post            `json:"-"`
}

func (meta *MetaPost) HTTPSOnly() bool {
	v := func(s string) bool { return !strings.HasPrefix(s, "https://") }
	for _, s := range meta.Servers {
		if v(s.URLs.OAuthAuth) || v(s.URLs.OAuthToken) || v(s.URLs.PostsFeed) || v(s.URLs.Post) || v(s.URLs.NewPost) ||
			v(s.URLs.PostAttachment) || v(s.URLs.Attachment) || v(s.URLs.Batch) || v(s.URLs.ServerInfo) {
			return false
		}
	}
	return true
}

type MetaProfile struct {
	Name     string `json:"name,omitempty"`
	Bio      string `json:"bio,omitempty"`
	Website  string `json:"website,omitempty"`
	Location string `json:"location,omitempty"`

	AvatarDigest string `json:"avatar_digest,omitempty"`
}

func GetMetaPost(url string) (*MetaPost, error) {
	post, err := GetPost(url)
	if err != nil {
		return nil, err
	}
	metaPost, err := ParseMeta(post.Post.Content, post.Post.Attachments)
	metaPost.Post = post.Post
	return metaPost, err
}

func ParseMeta(content []byte, attachments []*PostAttachment) (*MetaPost, error) {
	meta := &MetaPost{}
	err := json.Unmarshal(content, meta)
	if len(attachments) > 0 {
		// TODO: make this more strict
		meta.Profile.AvatarDigest = attachments[0].Digest
	}
	return meta, err
}

func getMetaPost(links []string, reqURL *url.URL) (*MetaPost, error) {
	for i, l := range links {
		// replace percent symbols with a private unicode character so that the url doesn't get decoded
		u, err := url.Parse(strings.Replace(l, "%", "\uFFFE", -1))
		if err != nil {
			return nil, err
		}
		m, err := GetMetaPost(strings.Replace(reqURL.ResolveReference(u).String(), "%EF%BF%BE", "%", -1))
		if err != nil && i < len(links)-1 {
			continue
		}
		return m, err
	}
	panic("not reached")
}
