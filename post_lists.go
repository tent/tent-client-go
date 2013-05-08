package tent

import (
	"net/http"
	"net/url"
	"strconv"
)

type PostVersionsPage struct {
	Versions []*PostVersion `json:"data"`
	Links    PageLinks      `json:"pages"`
	Header   PageHeader     `json:"-"`
}

func (client *Client) GetPostVersions(entity, post string, r *PageRequest) (*PostVersionsPage, error) {
	page := &PostVersionsPage{}
	return page, client.getPostListPage(entity, post, "", MediaTypePostVersions, r, page)
}

func (client *Client) GetPostChildren(entity, post, version string, r *PageRequest) (*PostVersionsPage, error) {
	page := &PostVersionsPage{}
	return page, client.getPostListPage(entity, post, version, MediaTypePostChildren, r, page)
}

func (f *PostVersionsPage) links() *PageLinks   { return &f.Links }
func (f *PostVersionsPage) header() *PageHeader { return &f.Header }

func (f *PostVersionsPage) First() (*PostVersionsPage, error) {
	page := &PostVersionsPage{}
	return page, f.Links.get(f.Links.First, page)
}

func (f *PostVersionsPage) Prev() (*PostVersionsPage, error) {
	page := &PostVersionsPage{}
	return page, f.Links.get(f.Links.Prev, page)
}

func (f *PostVersionsPage) Next() (*PostVersionsPage, error) {
	page := &PostVersionsPage{}
	return page, f.Links.get(f.Links.Next, page)
}

func (f *PostVersionsPage) Last() (*PostVersionsPage, error) {
	page := &PostVersionsPage{}
	return page, f.Links.get(f.Links.Last, page)
}

type PostMentionsPage struct {
	Mentions []*PostMention `json:"data"`
	Links    PageLinks      `json:"pages"`
	Header   PageHeader     `json:"-"`
}

func (client *Client) getPostListPage(entity, post, version, mediaType string, r *PageRequest, data pageType) error {
	header := make(http.Header)
	header.Set("Accept", mediaType)
	if r != nil && r.ETag != "" {
		header.Set("If-None-Match", r.ETag)
	}
	limit := 0
	if r != nil {
		limit = r.Limit
	}
	urlFunc := func(server *MetaPostServer) (string, error) {
		pu, err := server.URLs.PostURL(entity, post, version)
		if err != nil {
			return "", err
		}
		u, err := url.Parse(pu)
		if err != nil {
			return "", err
		}
		data.links().baseURL = u
		if limit > 0 {
			query := u.Query()
			query.Set("limit", strconv.Itoa(limit))
			u.RawQuery = query.Encode()
		}
		return u.String(), nil
	}
	if r != nil && r.ETag != "" {
		header.Set("If-None-Match", r.ETag)
	}
	if r != nil && r.CountOnly {
		var err error
		*data.header(), err = client.requestCount(urlFunc, header)
		return err
	}
	resHeader, err := client.requestJSON("GET", urlFunc, header, nil, data)
	if err != nil {
		if resErr, ok := err.(*BadResponseError); ok && resErr.Type == ErrBadStatusCode && resErr.Response.StatusCode == 304 {
			data.header().ETag = resHeader.Get("Etag")
			data.header().NotModified = true
			return nil
		}
		return err
	}
	data.links().client = client
	data.links().accept = mediaType
	data.header().ETag = resHeader.Get("Etag")
	return nil
}

func (client *Client) GetPostMentions(entity, post string, r *PageRequest) (*PostMentionsPage, error) {
	page := &PostMentionsPage{}
	return page, client.getPostListPage(entity, post, "", MediaTypePostMentions, r, page)
}

func (f *PostMentionsPage) links() *PageLinks   { return &f.Links }
func (f *PostMentionsPage) header() *PageHeader { return &f.Header }

func (f *PostMentionsPage) First() (*PostMentionsPage, error) {
	page := &PostMentionsPage{}
	return page, f.Links.get(f.Links.First, page)
}

func (f *PostMentionsPage) Prev() (*PostMentionsPage, error) {
	page := &PostMentionsPage{}
	return page, f.Links.get(f.Links.Prev, page)
}

func (f *PostMentionsPage) Next() (*PostMentionsPage, error) {
	page := &PostMentionsPage{}
	return page, f.Links.get(f.Links.Next, page)
}

func (f *PostMentionsPage) Last() (*PostMentionsPage, error) {
	page := &PostMentionsPage{}
	return page, f.Links.get(f.Links.Last, page)
}
