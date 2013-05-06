package tent

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
)

var ErrNoPage = errors.New("tent: the requested page does not exist")

type PageLinks struct {
	First string `json:"first"`
	Prev  string `json:"prev"`
	Next  string `json:"next"`
	Last  string `json:"last"`

	accept  string
	baseURL *url.URL
	client  *Client
}

func (links *PageLinks) get(query string, data linker) error {
	if query == "" {
		return ErrNoPage
	}
	links.baseURL.RawQuery = query[1:]
	header := make(http.Header)
	header.Set("Accept", links.accept)
	_, err := links.client.requestJSONURL("GET", links.baseURL.String(), header, nil, data)
	if err != nil {
		return err
	}
	l := data.links()
	l.accept = links.accept
	l.baseURL = links.baseURL
	l.client = links.client
	return nil
}

type linker interface {
	links() *PageLinks
}

type PostsFeedPage struct {
	Posts       []*Post   `json:"data"`
	Links       PageLinks `json:"pages"`
	ETag        string    `json:"-"`
	Count       int       `json:"-"`
	NotModified bool      `json:"-"`
}

type PageRequest struct {
	ETag      string
	CountOnly bool
}

func (client *Client) GetFeed(q *PostsFeedQuery, r *PageRequest) (*PostsFeedPage, error) {
	data := &PostsFeedPage{}
	header := make(http.Header)
	header.Set("Accept", MediaTypePostsFeed)
	if r != nil && r.ETag != "" {
		header.Set("If-None-Match", r.ETag)
	}
	urlFunc := func(server *MetaPostServer) (string, error) {
		u, err := url.Parse(server.URLs.PostsFeed)
		if err != nil {
			return "", err
		}
		data.Links.baseURL = u
		if q != nil {
			query := u.Query()
			for k, v := range q.Values {
				query[k] = v
			}
			u.RawQuery = query.Encode()
		}
		return u.String(), nil
	}
	if r != nil && r.CountOnly {
		err := client.Request(func(server *MetaPostServer) error {
			url, err := urlFunc(server)
			if err != nil {
				return err
			}
			req, err := client.newRequest("HEAD", url, header, nil)
			if err != nil {
				return err
			}
			res, err := HTTP.Do(req)
			if res.StatusCode == 304 {
				data.ETag = res.Header.Get("Etag")
				data.NotModified = true
				return nil
			}
			if res.StatusCode != 200 {
				return newBadResponseError(ErrBadStatusCode, res)
			}
			data.Count, _ = strconv.Atoi(res.Header.Get("Count"))
			return nil
		})
		return data, err
	}
	resHeader, err := client.requestJSON("GET", urlFunc, header, nil, data)
	if err != nil {
		if resErr, ok := err.(*BadResponseError); ok && resErr.Type == ErrBadStatusCode && resErr.Response.StatusCode == 304 {
			data.ETag = resHeader.Get("Etag")
			data.NotModified = true
			return data, nil
		}
		return nil, err
	}
	data.Links.client = client
	data.Links.accept = MediaTypePostsFeed
	data.ETag = resHeader.Get("Etag")
	return data, nil
}

func (f *PostsFeedPage) links() *PageLinks { return &f.Links }

func (f *PostsFeedPage) First() (*PostsFeedPage, error) {
	page := &PostsFeedPage{}
	return page, f.Links.get(f.Links.First, page)
}

func (f *PostsFeedPage) Prev() (*PostsFeedPage, error) {
	page := &PostsFeedPage{}
	return page, f.Links.get(f.Links.Prev, page)
}

func (f *PostsFeedPage) Next() (*PostsFeedPage, error) {
	page := &PostsFeedPage{}
	return page, f.Links.get(f.Links.Next, page)
}

func (f *PostsFeedPage) Last() (*PostsFeedPage, error) {
	page := &PostsFeedPage{}
	return page, f.Links.get(f.Links.Last, page)
}
