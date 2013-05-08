package tent

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/tent/hawk-go"
	"github.com/tent/http-link-go"
)

type Client struct {
	Credentials *hawk.Credentials

	Servers []MetaPostServer
}

const (
	MediaTypePost         = "application/vnd.tent.post.v0+json"
	MediaTypePostsFeed    = "application/vnd.tent.posts-feed.v0+json"
	MediaTypePostVersions = "application/vnd.tent.post-versions.v0+json"
	MediaTypePostMentions = "application/vnd.tent.post-mentions.v0+json"
	MediaTypePostChildren = "application/vnd.tent.post-children.v0+json"
)

func (client *Client) CreatePost(post *Post) error {
	defer post.initAttachments(client)
	if post.hasNewAttachments() {
		return client.createPostWithAttachments(post)
	}
	return client.createPost(post)
}

func (client *Client) createPostWithAttachments(post *Post) error {
	method, uri, err := client.postCreateURL(post)
	if err != nil {
		return err
	}
	req, err := client.newRequest(method, uri, nil, nil)
	if err != nil {
		return err
	}

	oldAttachments := make([]*PostAttachment, 0, len(post.Attachments))
	newAttachments := make([]*PostAttachment, 0, len(post.Attachments))
	for _, att := range post.Attachments {
		if att.Data != nil {
			newAttachments = append(newAttachments, att)
		} else {
			oldAttachments = append(oldAttachments, att)
		}
	}
	post.Attachments = oldAttachments

	bodyReader, bodyWriter := io.Pipe()
	postWriter := NewMultipartPostWriter(bodyWriter)
	req.Header.Set("Content-Type", postWriter.ContentType())
	req.Body = bodyReader
	errChan := make(chan error, 1)
	go func() {
		defer bodyWriter.Close()
		err = postWriter.WritePost(post)
		if err != nil {
			errChan <- err
			return
		}
		for _, att := range newAttachments {
			err = postWriter.WriteAttachment(att)
			if err != nil {
				errChan <- err
				return
			}
		}
		err = postWriter.Close()
		errChan <- err
	}()

	res, err := HTTP.Do(req)
	if err != nil {
		return err
	}
	if err = <-errChan; err != nil {
		return err
	}

	return parsePostCreateRes(post, res)
}

func (client *Client) createPost(post *Post) error {
	data, err := json.Marshal(post)
	if err != nil {
		return err
	}
	method, uri, err := client.postCreateURL(post)
	if err != nil {
		return err
	}
	header := make(http.Header)
	header.Set("Content-Type", post.contentType())
	if len(post.Links) > 0 {
		header.Set("Link", link.Format(post.Links))
		post.Links = nil
	}
	req, err := client.newRequest(method, uri, header, data)
	if err != nil {
		return err
	}
	res, err := HTTP.Do(req)
	if err != nil {
		return err
	}
	return parsePostCreateRes(post, res)
}

func parsePostCreateRes(post *Post, res *http.Response) error {
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return newBadResponseError(ErrBadStatusCode, res)
	}

	if linkHeader := res.Header.Get("Link"); linkHeader != "" {
		links, err := link.Parse(linkHeader)
		if err != nil {
			return err
		}
		post.Links = links
	}

	var err error
	if ok := timeoutRead(res.Body, func() {
		err = json.NewDecoder(res.Body).Decode(post)
	}); !ok {
		return newBadResponseError(ErrReadTimeout, res)
	}
	return err
}

func (client *Client) postCreateURL(post *Post) (method string, uri string, err error) {
	if post.ID == "" {
		method, uri = "POST", client.Servers[0].URLs.NewPost
	} else {
		method = "PUT"
		uri, err = client.Servers[0].URLs.PostURL(post.Entity, post.ID, "")
		if err != nil {
			return
		}
		post.Entity = ""
		post.ID = ""
	}
	return
}

func (client *Client) GetAttachment(entity, digest string) (body io.ReadCloser, err error) {
	err = client.Request(func(server *MetaPostServer) error {
		url := server.URLs.AttachmentURL(entity, digest)
		req, err := client.newRequest("GET", url, nil, nil)
		if err != nil {
			return err
		}
		res, err := HTTP.Do(req)
		if err != nil {
			return err
		}
		if res.StatusCode != 200 {
			return newBadResponseError(ErrBadStatusCode, res)
		}
		body = res.Body
		return nil
	})
	return
}

func (client *Client) GetPostAttachment(entity, post, version, name, accept string) (body io.ReadCloser, err error) {
	err = client.Request(func(server *MetaPostServer) error {
		if version == "" {
			version = "latest"
		}
		url := server.URLs.PostAttachmentURL(entity, post, version, name)
		req, err := client.newRequest("GET", url, nil, nil)
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		if err != nil {
			return err
		}
		res, err := HTTP.Do(req)
		if err != nil {
			return err
		}
		if res.StatusCode != 200 {
			return newBadResponseError(ErrBadStatusCode, res)
		}
		body = res.Body
		return nil
	})
	return
}

func (client *Client) Request(req func(*MetaPostServer) error) error {
	for i, server := range client.Servers {
		err := req(&server)
		if err != nil && i < len(client.Servers)-1 {
			continue
		}
		return err
	}
	panic("not reached")
}

func (client *Client) SignRequest(req *http.Request, body []byte) {
	if client.Credentials == nil {
		panic("tent: missing credentials")
	}
	auth := hawk.NewRequestAuth(req, client.Credentials, 0)
	if body != nil {
		mediaType, _, _ := mime.ParseMediaType(req.Header.Get("Content-Type"))
		h := auth.PayloadHash(mediaType)
		h.Write(body)
		auth.SetHash(h)
	}
	req.Header.Set("Authorization", auth.RequestHeader())
}

func (client *Client) newRequest(method, url string, header http.Header, body []byte) (*http.Request, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := newRequest(method, url, header, bodyReader)
	if err != nil {
		return nil, err
	}
	if client.Credentials != nil {
		client.SignRequest(req, body)
	}
	return req, nil
}

func (client *Client) requestJSON(method string, url urlFunc, reqHeader http.Header, body []byte, data interface{}) (header http.Header, err error) {
	return header, client.Request(func(server *MetaPostServer) error {
		uri, err := url(server)
		if err != nil {
			return err
		}
		header, err = client.requestJSONURL(method, uri, reqHeader, body, data)
		return err
	})
}

func (client *Client) requestJSONURL(method string, url string, header http.Header, body []byte, data interface{}) (http.Header, error) {
	req, err := client.newRequest(method, url, header, body)
	if err != nil {
		return nil, err
	}
	res, err := HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, newBadResponseError(ErrBadStatusCode, res)
	}
	if ok := timeoutRead(res.Body, func() {
		err = json.NewDecoder(res.Body).Decode(data)
	}); !ok {
		return nil, newBadResponseError(ErrReadTimeout, res)
	}
	return res.Header, err
}

type urlFunc func(server *MetaPostServer) (string, error)

func (client *Client) requestCount(urlFunc urlFunc, header http.Header) (PageHeader, error) {
	h := PageHeader{}
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
		if err != nil {
			return err
		}
		res.Body.Close()
		if res.StatusCode == 304 {
			h.ETag = res.Header.Get("Etag")
			h.NotModified = true
			return nil
		}
		if res.StatusCode != 200 {
			return newBadResponseError(ErrBadStatusCode, res)
		}
		h.Count, _ = strconv.Atoi(res.Header.Get("Count"))
		return nil
	})
	return h, err
}

var timeout = 10 * time.Second
var HTTP = newHTTPClient(timeout)
var UserAgent = "tent-go/1"

func SetTimeout(t time.Duration) {
	timeout = t
	HTTP = newHTTPClient(t)
}

func newHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			ResponseHeaderTimeout: timeout,
			Dial: (&net.Dialer{Timeout: timeout}).Dial,
		},
	}
}

func timeoutRead(body io.Closer, read func()) (ok bool) {
	done := make(chan struct{})
	go func() {
		read()
		done <- struct{}{}
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		body.Close()
		return false
	}
}

func newRequest(method, url string, header http.Header, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	if req.URL.Path != "" {
		// url.Parse unescapes the Path, which can screw with some of the signing stuff
		req.URL.Opaque = strings.SplitN(url, ":", 2)[1]
		req.URL.Path = ""
		req.URL.RawQuery = ""
	}
	if header != nil {
		req.Header = header
	}
	req.Header.Set("User-Agent", UserAgent)
	return req, nil
}

type BadResponseErrorType int

const (
	ErrBadStatusCode BadResponseErrorType = iota
	ErrBadContentType
	ErrReadTimeout
)

type BadResponseError struct {
	Type      BadResponseErrorType
	Response  *http.Response
	TentError *TentError
}

type TentError struct {
	Error  string            `json:"error"`
	Fields map[string]string `json:"fields"`
}

const MediaTypeError = "application/vnd.tent.error.v0+json"

func newBadResponseError(typ BadResponseErrorType, res *http.Response) *BadResponseError {
	err := &BadResponseError{Type: typ, Response: res}

	// try to decode an error message from the body
	if typ == ErrBadStatusCode {
		mediaType, _, _ := mime.ParseMediaType(res.Header.Get("Content-Type"))
		if mediaType == MediaTypeError {
			tentError := &TentError{}
			json.NewDecoder(res.Body).Decode(tentError)
			err.TentError = tentError
		}
	}
	return err
}

func (e *BadResponseError) Error() string {
	switch e.Type {
	case ErrBadContentType:
		return "tent: incorrect Content-Type received: " + strconv.Quote(e.Response.Header.Get("Content-Type"))
	case ErrReadTimeout:
		return "tent: timeout reading response body of " + e.Response.Request.Method + " " + e.Response.Request.URL.String()
	default:
		return "tent: unexpected " + strconv.Itoa(e.Response.StatusCode) + " performing " + e.Response.Request.Method + " " + e.Response.Request.URL.String()
	}
}
