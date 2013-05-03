package tent

import (
	"errors"
	"net/http"
	"net/url"
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
	err := links.client.requestJSONURL("GET", links.baseURL.String(), header, nil, data)
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
	Posts []*Post   `json:"data"`
	Links PageLinks `json:"pages"`
}

func (client *Client) GetFeed(q *PostsFeedQuery) (*PostsFeedPage, error) {
	data := &PostsFeedPage{}
	header := make(http.Header)
	header.Set("Accept", MediaTypePostsFeed)
	err := client.requestJSON("GET", func(server *MetaPostServer) (string, error) {
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
	}, header, nil, data)
	if err != nil {
		return nil, err
	}
	data.Links.client = client
	data.Links.accept = MediaTypePostsFeed
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
