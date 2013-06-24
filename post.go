package tent

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/cupcake/sfilter"
	"github.com/tent/canonical-json-go"
	"github.com/tent/hawk-go"
	"github.com/tent/http-link-go"
)

type PostRef struct {
	Entity         string `json:"entity,omitempty" sfilter:"version"`
	OriginalEntity string `json:"original_entity,omitempty"`
	Post           string `json:"post,omitempty" sfilter:"version"`
	Version        string `json:"version,omitempty" sfilter:"version"`
	Type           string `json:"type,omitempty" sfilter:"version"`
}

type PostMention struct {
	Entity         string `json:"entity,omitempty" sfilter:"version"`
	OriginalEntity string `json:"original_entity,omitempty"`
	Post           string `json:"post,omitempty" sfilter:"version"`
	Version        string `json:"version,omitempty" sfilter:"version"`
	Type           string `json:"type,omitempty" sfilter:"version"`
	PublicFlag     *bool  `json:"public,omitempty"` // nil or true is public == true; false is public == false
}

func (mention *PostMention) Public() bool {
	return mention.PublicFlag == nil || *mention.PublicFlag
}

type PostAttachment struct {
	Name        string `json:"name" sfilter:"version"`
	Category    string `json:"category" sfilter:"version"`
	ContentType string `json:"content_type" sfilter:"version"`
	Size        int64  `json:"size,omitempty" sfilter:"version"`
	Digest      string `json:"digest,omitempty" sfilter:"version"`

	// Include Data to upload a new attachment with the post
	Data ReadLenSeeker `json:"-"`

	entity string `json:"-"`
	body   io.ReadCloser
	client *Client
}

// Read downloads and reads from the attachment body.
// The attachment must have be initialized by downloading the containing post
// from the server in order to use Read.
func (att *PostAttachment) Read(p []byte) (int, error) {
	if att.client == nil || att.Digest == "" || att.entity == "" {
		return 0, errors.New("tent: improperly initialized attachment")
	}
	if att.body == nil {
		var err error
		att.body, err = att.client.GetAttachment(att.entity, att.Digest)
		if err != nil {
			return 0, err
		}
	}
	return att.body.Read(p)
}

func (att *PostAttachment) Close() error {
	if att.body == nil {
		return nil
	}
	return att.body.Close()
}

type PostPermissions struct {
	PublicFlag *bool    `json:"public,omitempty"` // nil or true is public == true; false is public == false
	Groups     []string `json:"groups,omitempty"`
	Entities   []string `json:"entities,omitempty"`
}

func (perm *PostPermissions) Public() bool {
	return perm == nil || perm.PublicFlag == nil || *perm.PublicFlag
}

type PostApp struct {
	Name string `json:"name,omitempty sfilter:"version""`
	URL  string `json:"url,omitempty" sfilter:"version"`
	ID   string `json:"id,omitempty"`
}

type PostVersionParent struct {
	Entity         string `json:"entity,omitempty" sfilter:"version"`
	OriginalEntity string `json:"original_entity,omitempty"`
	Post           string `json:"post,omitempty" sfilter:"version"`
	Version        string `json:"version" sfilter:"version"`
}

type PostVersion struct {
	ID          string              `json:"id,omitempty" sfilter:"version"`
	Parents     []PostVersionParent `json:"parents,omitempty" sfilter:"version"`
	Message     string              `json:"message,omitempty" sfilter:"version"`
	PublishedAt *UnixTime           `json:"published_at,omitempty" sfilter:"version"`
	ReceivedAt  *UnixTime           `json:"received_at,omitempty"`

	// Used in post version and children lists
	Type   string `json:"type,omitempty"`
	Entity string `json:"entity,omitempty"`
	Post   string `json:"post,omitempty"`
}

type PostLicense struct {
	URL string `json:"url" sfilter:"version"`
}

type Post struct {
	ID string `json:"id,omitempty" sfilter:"version"`

	Entity         string `json:"entity,omitempty" sfilter:"version"`
	OriginalEntity string `json:"original_entity,omitempty"`

	Type    string          `json:"type" sfilter:"version"`
	Content json.RawMessage `json:"content,omitempty" sfilter:"version"`

	Version *PostVersion `json:"version,omitempty" sfilter:"version"`

	Refs        []PostRef         `json:"refs,omitempty" sfilter:"version"`
	Mentions    []PostMention     `json:"mentions,omitempty" sfilter:"version"`
	Licenses    []PostLicense     `json:"licenses,omitempty" sfilter:"version"`
	Attachments []*PostAttachment `json:"attachments,omitempty" sfilter:"version"`
	Permissions *PostPermissions  `json:"permissions,omitempty"`

	App *PostApp `json:"app,omitempty" sfilter:"version"`

	ReceivedAt  *UnixTime `json:"received_at,omitempty"`
	PublishedAt *UnixTime `json:"published_at,omitempty" sfilter:"version"`

	Links []link.Link `json:"-"`

	Notification bool `json:"-"`
}

type PostEnvelope struct {
	Post *Post  `json:"post"`
	Refs []Post `json:"refs"`
}

const RelCredentials = "https://tent.io/rels/credentials"

var ErrMissingCredentialsLink = errors.New("tent: missing credentials link")

type PostRequest struct {
	MaxRefs int
}

func (client *Client) GetPost(entity, id, version string, r *PostRequest) (*PostEnvelope, error) {
	post := &PostEnvelope{}
	header := make(http.Header)
	header.Set("Accept", MediaTypePost)
	urlFunc := func(server *MetaPostServer) (string, error) {
		u, err := server.URLs.PostURL(entity, id, version)
		if err != nil {
			return "", err
		}
		if r != nil && r.MaxRefs > 0 {
			if strings.Contains(u, "?") {
				uri, err := url.Parse(u)
				if err != nil {
					return "", err
				}
				q := uri.Query()
				q.Add("max_refs", strconv.Itoa(r.MaxRefs))
				uri.RawQuery = q.Encode()
				u = uri.String()
			} else {
				u += "?max_refs=" + strconv.Itoa(r.MaxRefs)
			}
		}
		return u, nil
	}
	_, err := client.requestJSON("GET", urlFunc, header, nil, post)
	if err != nil || post.Post == nil {
		if err == nil {
			err = newBadResponseError(ErrBadData, nil)
		}
		return nil, err
	}
	post.Post.initAttachments(client)
	for _, p := range post.Refs {
		p.initAttachments(client)
	}
	return post, err
}

func GetPost(url string) (*PostEnvelope, error) {
	req, err := newRequest("GET", url, nil, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", MediaTypePost)
	res, err := HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, newBadResponseError(ErrBadStatusCode, res)
	}

	post := &PostEnvelope{}
	if ok := timeoutRead(res.Body, func() {
		err = json.NewDecoder(res.Body).Decode(post)
	}); !ok {
		return nil, newBadResponseError(ErrReadTimeout, res)
	}
	if post.Post == nil {
		return nil, newBadResponseError(ErrBadData, res)
	}
	c := &Client{}
	post.Post.initAttachments(c)
	for _, p := range post.Refs {
		p.initAttachments(c)
	}
	return post, err
}

func (post *Post) LinkedCredentials() (*hawk.Credentials, *Post, error) {
	var credsPostURL string
	for _, l := range post.Links {
		if l.Rel == RelCredentials {
			credsPostURL = l.URI
			break
		}
	}
	if credsPostURL == "" {
		return nil, nil, ErrMissingCredentialsLink
	}
	p, err := GetPost(credsPostURL)
	if err != nil {
		return nil, nil, err
	}
	creds, err := ParseCredentials(p.Post)
	return creds, p.Post, err
}

func (post *Post) hasNewAttachments() bool {
	for _, att := range post.Attachments {
		if att.Data != nil {
			return true
		}
	}
	return false
}

func (post *Post) CalculateVersion() (string, error) {
	data, _ := sfilter.Map(post, "version")

	if post.OriginalEntity != "" {
		data["entity"] = post.OriginalEntity
	}

	if data["mentions"] != nil {
		postMentions := data["mentions"].([]map[string]interface{})
		mentions := make([]map[string]interface{}, 0, len(postMentions))
		for i, mention := range post.Mentions {
			if !mention.Public() {
				continue
			}
			if mention.Entity == post.Entity {
				delete(postMentions[i], "entity")
			}
			if mention.Post == post.ID {
				delete(postMentions[i], "post")
			}
			if mention.OriginalEntity != "" && post.OriginalEntity != mention.OriginalEntity {
				postMentions[i]["entity"] = mention.OriginalEntity
			}
			if len(postMentions[i]) == 0 {
				continue
			}
			mentions = append(mentions, postMentions[i])
		}
		data["mentions"] = mentions
		if len(mentions) == 0 {
			delete(data, "mentions")
		}
	}

	if data["refs"] != nil {
		refs := data["refs"].([]map[string]interface{})
		for i, ref := range post.Refs {
			if ref.Entity == post.Entity {
				delete(refs[i], "entity")
			}
			if ref.Post == post.ID {
				delete(refs[i], "post")
			}
			if ref.OriginalEntity != "" && post.OriginalEntity != ref.OriginalEntity {
				refs[i]["entity"] = ref.OriginalEntity
			}
		}
	}

	if len(post.Version.Parents) > 0 {
		version := data["version"].(map[string]interface{})
		parents := version["parents"].([]map[string]interface{})
		for i, parent := range post.Version.Parents {
			if parent.Entity == post.Entity {
				delete(parents[i], "entity")
			}
			if parent.Post == post.ID {
				delete(parents[i], "post")
			}
			if parent.OriginalEntity != "" && post.OriginalEntity != parent.OriginalEntity {
				parents[i]["entity"] = parent.OriginalEntity
			}
		}
	}

	if len(post.Content) < 3 {
		delete(data, "content")
	}

	canonicalJSON, err := cjson.Marshal(data)
	if err != nil {
		return "", err
	}

	h := sha512.New()
	h.Write(canonicalJSON)
	return hex.EncodeToString(h.Sum(nil)[:32]), nil
}

func (post *Post) contentType() string {
	params := map[string]string{"type": post.Type}
	if post.Notification {
		params["rel"] = "https://tent.io/rels/notification"
	}
	return mime.FormatMediaType(MediaTypePost, params)
}

func (post *Post) initAttachments(client *Client) {
	if post == nil {
		return
	}
	for _, att := range post.Attachments {
		att.entity = post.Entity
		att.client = client
	}
}

func SplitType(typ string) (base string, fragment string) {
	s := strings.SplitN(typ, "#", 2)
	base = s[0]
	if len(s) > 1 {
		fragment = s[1]
	}
	return
}

func TypeBase(typ string) string {
	base, _ := SplitType(typ)
	return base
}

func ParseCredentials(post *Post) (*hawk.Credentials, error) {
	creds := &hawk.Credentials{ID: post.ID, Hash: sha256.New}
	temp := &Credentials{}
	err := json.Unmarshal(post.Content, temp)
	creds.Key = temp.HawkKey
	for _, mention := range post.Mentions {
		if mention.Type == PostTypeApp {
			creds.App = mention.Post
		}
	}
	return creds, err
}
