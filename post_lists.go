package tent

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
)

type PageHeader struct {
	ETag        string
	Count       int
	NotModified bool
}

type PageRequest struct {
	ETag      string
	CountOnly bool
	Limit     int
}

type PostListPage struct {
	Mentions []*PostMention `json:"mentions,omitempty"`
	Versions []*PostVersion `json:"versions,omitempty"`
	Posts    []*Post        `json:"posts,omitempty"`
	Links    PageLinks      `json:"pages"`
	Header   PageHeader     `json:"-"`
}

type PageLinks struct {
	First string `json:"first,omitempty"`
	Prev  string `json:"prev,omitempty"`
	Next  string `json:"next,omitempty"`
	Last  string `json:"last,omitempty"`

	accept  string
	baseURL *url.URL
	client  *Client
}

func (links *PageLinks) get(query string) (*PostListPage, error) {
	if query == "" {
		return nil, ErrNoPage
	}
	page := &PostListPage{Links: PageLinks{accept: links.accept, baseURL: links.baseURL, client: links.client}}
	links.baseURL.RawQuery = query[1:]
	header := make(http.Header)
	header.Set("Accept", links.accept)
	_, err := links.client.requestJSONURL("GET", links.baseURL.String(), header, nil, page)
	if err != nil {
		return nil, err
	}
	return page, nil
}

func (client *Client) GetFeed(q *PostsFeedQuery, r *PageRequest) (*PostListPage, error) {
	return client.getPostListPage("", "", "", MediaTypePostsFeed, r, q.Values)
}

func (client *Client) GetVersions(entity, post string, r *PageRequest) (*PostListPage, error) {
	return client.getPostListPage(entity, post, "", MediaTypePostVersions, r, nil)
}

func (client *Client) GetChildren(entity, post, version string, r *PageRequest) (*PostListPage, error) {
	return client.getPostListPage(entity, post, version, MediaTypePostChildren, r, nil)
}

func (client *Client) GetMentions(entity, post string, r *PageRequest) (*PostListPage, error) {
	return client.getPostListPage(entity, post, "", MediaTypePostMentions, r, nil)
}

var ErrNoPage = errors.New("tent: the requested page does not exist")

func (client *Client) getPostListPage(entity, post, version, mediaType string, r *PageRequest, query url.Values) (*PostListPage, error) {
	header := make(http.Header)
	header.Set("Accept", mediaType)
	if r != nil && r.ETag != "" {
		header.Set("If-None-Match", r.ETag)
	}
	limit := 0
	if r != nil {
		limit = r.Limit
	}
	page := &PostListPage{}
	urlFunc := func(server *MetaPostServer) (string, error) {
		var pu string
		var err error
		if mediaType == MediaTypePostsFeed {
			pu = server.URLs.PostsFeed
		} else {
			pu, err = server.URLs.PostURL(entity, post, version)
		}
		if err != nil {
			return "", err
		}
		u, err := url.Parse(pu)
		if err != nil {
			return "", err
		}
		page.Links.baseURL = u
		if len(query) > 0 || limit > 0 {
			uq := u.Query()
			for k, v := range query {
				uq[k] = v
			}
			if limit > 0 {
				uq.Set("limit", strconv.Itoa(limit))
			}
			u.RawQuery = uq.Encode()
		}
		return u.String(), nil
	}
	if r != nil && r.ETag != "" {
		header.Set("If-None-Match", r.ETag)
	}
	if r != nil && r.CountOnly {
		var err error
		page.Header, err = client.requestCount(urlFunc, header)
		return nil, err
	}
	resHeader, err := client.requestJSON("GET", urlFunc, header, nil, page)
	if err != nil {
		if resErr, ok := err.(*ResponseError); ok && resErr.Type == ErrBadStatusCode && resErr.Response.StatusCode == 304 {
			page.Header.ETag = resHeader.Get("Etag")
			page.Header.NotModified = true
			return page, nil
		}
		return nil, err
	}
	page.Links.client = client
	page.Links.accept = mediaType
	page.Header.ETag = resHeader.Get("Etag")
	return page, nil
}

func (f *PostListPage) First() (*PostListPage, error) { return f.Links.get(f.Links.First) }
func (f *PostListPage) Prev() (*PostListPage, error)  { return f.Links.get(f.Links.Prev) }
func (f *PostListPage) Next() (*PostListPage, error)  { return f.Links.get(f.Links.Next) }
func (f *PostListPage) Last() (*PostListPage, error)  { return f.Links.get(f.Links.Last) }
