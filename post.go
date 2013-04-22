package tent

import (
	"encoding/json"
	"errors"

	"github.com/tent/http-link-go"
)

type PostMention struct {
	Entity         string `json:"entity,omitempty"`
	OriginalEntity string `json:"original_entity,omitempty"`
	Post           string `json:"post,omitempty"`
	Version        string `json:"version,omitempty"`
	Type           string `json:"type,omitempty"`
	Public         bool   `json:"public"`
}

type PostAttachment struct {
	Name        string `json:"name"`
	Category    string `json:"category"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	Digest      string `json:"digest"`
}

type PostPermissions struct {
	Public   bool     `json:"public"`
	Groups   []string `json:"groups,omitempty"`
	Entities []string `json:"entities,omitempty"`
}

type PostApp struct {
	Name string `json:"name,omitempty"`
	URL  string `json:"url,omitempty"`
	ID   string `json:"id,omitempty"`
}

type PostVersionParent struct {
	Entity         string `json:"entity,omitempty"`
	OriginalEntity string `json:"original_entity,omitempty"`
	Post           string `json:"post,omitempty"`
	Version        string `json:"version"`
}

type PostVersion struct {
	ID          string              `json:"id,omitempty"`
	Parents     []PostVersionParent `json:"parents,omitempty"`
	Message     string              `json:"message,omitempty"`
	PublishedAt UnixTime            `json:"published_at"`
	ReceivedAt  UnixTime            `json:"received_at"`

	// Used in post version and children lists
	Type   string `json:"type,omitempty"`
	Entity string `json:"entity,omitempty"`
	Post   string `json:"post,omitempty"`
}

type Post struct {
	ID string `json:"id"`

	Entity         string `json:"entity"`
	OriginalEntity string `json:"original_entity,omitempty"`

	Type    string          `json:"type"`
	Content json.RawMessage `json:"content,omitempty"`

	Version PostVersion `json:"version"`

	Mentions    []PostMention    `json:"mentions,omitempty"`
	Licenses    []string         `json:"licenses,omitempty"`
	Attachments []PostAttachment `json:"attachments,omitempty"`
	Permissions PostPermissions  `json:"permissions"`

	App PostApp `json:"app,omitempty"`

	ReceivedAt  UnixTime `json:"received_at"`
	PublishedAt UnixTime `json:"published_at"`

	Links []link.Link `json:"-"`
}

const RelCredentials = "https://tent.io/rels/credentials"

var ErrMissingCredentialsLink = errors.New("tent: missing credentials link")

func (post *Post) GetCredentials() (*Post, error) {
	var credsPostURL string
	for _, l := range post.Links {
		if l.Params["rel"] == RelCredentials {
			credsPostURL = l.URL
			break
		}
	}
	if credsPostURL == "" {
		return nil, ErrMissingCredentialsLink
	}

	req, err := newRequest("GET", credsPostURL, nil)
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

	credsPost := &Post{}
	if ok := timeoutRead(res.Body, func() {
		err = json.NewDecoder(res.Body).Decode(credsPost)
	}); !ok {
		return nil, &BadResponseError{ErrReadTimeout, res}
	}
	return credsPost, err
}
