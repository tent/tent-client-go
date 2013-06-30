package tent

import (
	"bytes"
	"io"
	"mime"

	"code.google.com/p/go.net/html"
	"github.com/tent/http-link-go"
)

func Discover(entity string) (*MetaPost, error) {
	req, err := newRequest("HEAD", entity, nil, nil)
	if req.URL.Path == "" {
		req.URL.Path = "/"
	}
	res, err := HTTP.Do(req)
	if err != nil {
		return nil, newRequestError(err, req)
	}
	res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, newResponseError(ErrBadStatusCode, res)
	}

	if linkHeader := res.Header.Get("Link"); linkHeader != "" {
		links, err := link.Parse(linkHeader)
		if err != nil {
			return nil, err
		}
		var metaLinks []string
		for _, l := range links {
			if l.Rel == RelMetaPost {
				metaLinks = append(metaLinks, l.URI)
			}
		}
		if len(metaLinks) > 0 {
			return getMetaPost(metaLinks, res.Request.URL)
		}
	}

	// we didn't get anything with the HEAD request, so let's try to GET HTML links
	req, _ = newRequest("GET", entity, nil, nil)
	req.Header.Set("Accept", "text/html")
	res, err = HTTP.Do(req)
	if err != nil {
		return nil, newRequestError(err, req)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, newResponseError(ErrBadStatusCode, res)
	}
	contentType := res.Header.Get("Content-Type")
	if contentType == "" {
		return nil, newResponseError(ErrBadContentType, res)
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, err
	}
	if mediaType != "text/html" {
		return nil, newResponseError(ErrBadContentType, res)
	}

	var links []string
	if ok := timeoutRead(res.Body, func() {
		links, err = parseHTMLMetaLinks(res.Body)
	}); !ok {
		return nil, newResponseError(ErrReadTimeout, res)
	}
	if err != nil {
		return nil, err
	}
	if len(links) > 0 {
		return getMetaPost(links, res.Request.URL)
	}

	return nil, nil
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
