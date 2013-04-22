package tent

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/url"
	"strings"

	"code.google.com/p/go.net/html"
	"github.com/tent/http-link-go"
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

func (urls *MetaPostServerURLs) PostURL(entity, post string) string {
	u := strings.Replace(urls.Post, "{entity}", url.QueryEscape(entity), 1)
	return strings.Replace(u, "{post}", post, 1)
}

func (urls *MetaPostServerURLs) PostAttachmentURL(entity, post, name, version string) string {
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
	req, err := newRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	res, err := HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, &BadResponseError{ErrBadStatusCode, res}
	}
	post := &Post{}
	if ok := timeoutRead(res.Body, func() {
		err = json.NewDecoder(res.Body).Decode(post)
	}); !ok {
		return nil, &BadResponseError{ErrReadTimeout, res}
	}
	if err != nil {
		return nil, err
	}
	metaPost := &MetaPost{Post: post}
	err = json.Unmarshal(post.Content, metaPost)
	return metaPost, err
}

func Discover(entity string) (*MetaPost, error) {
	req, err := newRequest("HEAD", entity, nil)
	res, err := HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, &BadResponseError{ErrBadStatusCode, res}
	}

	if linkHeader := res.Header.Get("Link"); linkHeader != "" {
		links, err := link.Parse(linkHeader)
		if err != nil {
			return nil, err
		}
		var metaLinks []string
		for _, l := range links {
			if l.Params["rel"] == RelMetaPost {
				metaLinks = append(metaLinks, l.URL)
			}
		}
		if len(metaLinks) > 0 {
			return getMetaPost(metaLinks, res.Request.URL)
		}
	}

	// we didn't get anything with the HEAD request, so let's try to GET HTML links
	req, _ = newRequest("GET", entity, nil)
	res, err = HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, &BadResponseError{ErrBadStatusCode, res}
	}
	contentType := res.Header.Get("Content-Type")
	if contentType == "" {
		return nil, &BadResponseError{ErrBadContentType, res}
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, err
	}
	if mediaType != "text/html" {
		return nil, &BadResponseError{ErrBadContentType, res}
	}

	var links []string
	if ok := timeoutRead(res.Body, func() {
		links, err = parseHTMLMetaLinks(res.Body)
	}); !ok {
		return nil, &BadResponseError{ErrReadTimeout, res}
	}
	if err != nil {
		return nil, err
	}
	if len(links) > 0 {
		return getMetaPost(links, res.Request.URL)
	}

	return nil, nil
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

func parseHTMLMetaLinks(data io.Reader) (links []string, err error) {
	t := html.NewTokenizer(data)
loop:
	for {
		switch t.Next() {
		case html.ErrorToken:
			err = t.Err()
			if err == io.EOF {
				err = nil
			}
			break loop
		case html.StartTagToken, html.SelfClosingTagToken:
			name, attrs := t.TagName()
			if !attrs || !bytes.Equal(name, []byte("link")) {
				continue loop
			}
			var href string
			var haveRel, metaRel bool
			for {
				key, val, more := t.TagAttr()
				if bytes.Equal(key, []byte("href")) {
					href = string(val)
				} else if bytes.Equal(key, []byte("rel")) {
					haveRel = true
					if bytes.Equal(val, []byte(RelMetaPost)) {
						metaRel = true
					}
				}
				if !more || haveRel && !metaRel || metaRel && href != "" {
					break
				}
			}
			if metaRel && href != "" {
				links = append(links, href)
			}
		}
	}
	return
}
