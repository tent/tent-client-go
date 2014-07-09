package tent

import (
	"bytes"
	"encoding/json"
	"fmt"
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

const (
	MediaTypePost         = "application/vnd.tent.post.v0+json"
	MediaTypePostsFeed    = "application/vnd.tent.posts-feed.v0+json"
	MediaTypePostVersions = "application/vnd.tent.post-versions.v0+json"
	MediaTypePostMentions = "application/vnd.tent.post-mentions.v0+json"
	MediaTypePostChildren = "application/vnd.tent.post-children.v0+json"
)

type Client struct {
	Credentials *hawk.Credentials

	Servers []MetaPostServer

	Entity string
}

func NewClient(credsPost *Post, metaContent []byte) (*Client, error) {
	creds, err := ParseCredentials(credsPost)
	if err != nil {
		return nil, err
	}
	meta, err := ParseMeta(metaContent, nil)
	if err != nil {
		return nil, err
	}
	return &Client{Credentials: creds, Servers: meta.Servers, Entity: meta.Entity}, nil
}

func (client *Client) CreatePost(post *Post) error {
	defer post.initAttachments(client)
	if post.hasNewAttachments() {
		return client.createPostWithAttachments(post)
	}
	return client.createPost(post)
}

func (client *Client) createPostWithAttachments(post *Post) error {
	method, uri := client.postCreateURL(post)
	req, err := client.NewRequest(method, uri, nil, nil)
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
		return newRequestError(err, req)
	}
	if err = <-errChan; err != nil {
		return newRequestError(err, req)
	}

	return parsePostRes(post, res)
}

func (client *Client) createPost(post *Post) error {
	data, err := json.Marshal(post)
	if err != nil {
		return err
	}
	method, uri := client.postCreateURL(post)
	header := make(http.Header)
	header.Set("Content-Type", post.contentType())
	if len(post.Links) > 0 {
		header.Set("Link", link.Format(post.Links))
		post.Links = nil
	}
	req, err := client.NewRequest(method, uri, header, data)
	if err != nil {
		return err
	}
	res, err := HTTP.Do(req)
	if err != nil {
		return newRequestError(err, req)
	}
	return parsePostRes(post, res)
}

func parsePostRes(post *Post, res *http.Response) error {
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return newResponseError(ErrBadStatusCode, res)
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
		err = json.NewDecoder(res.Body).Decode(&PostEnvelope{Post: post})
	}); !ok {
		return newResponseError(ErrReadTimeout, res)
	}
	return err
}

func (client *Client) postCreateURL(post *Post) (method string, uri string) {
	if post.ID == "" {
		method, uri = "POST", client.Servers[0].URLs.NewPost
	} else {
		method = "PUT"
		uri = client.Servers[0].URLs.PostURL(post.Entity, post.ID, "")
		post.Entity = ""
		post.ID = ""
	}
	return
}

func (client *Client) GetAttachment(entity, digest string) (body io.ReadCloser, header http.Header, err error) {
	err = client.Request(func(server *MetaPostServer) error {
		url := server.URLs.AttachmentURL(entity, digest)
		req, err := client.NewRequest("GET", url, nil, nil)
		if err != nil {
			return err
		}
		res, err := HTTP.Do(req)
		if err != nil {
			return newRequestError(err, req)
		}
		if res.StatusCode != 200 {
			return newResponseError(ErrBadStatusCode, res)
		}
		body = res.Body
		header = res.Header
		return nil
	})
	return
}

func (client *Client) GetPostAttachment(entity, post, version, name, accept string) (body io.ReadCloser, header http.Header, err error) {
	err = client.Request(func(server *MetaPostServer) error {
		url := server.URLs.PostAttachmentURL(entity, post, version, name)
		req, err := client.NewRequest("GET", url, nil, nil)
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		if err != nil {
			return err
		}
		res, err := HTTP.Do(req)
		if err != nil {
			return newRequestError(err, req)
		}
		if res.StatusCode != 200 {
			return newResponseError(ErrBadStatusCode, res)
		}
		body = res.Body
		header = res.Header
		return nil
	})
	return
}

func (client *Client) DeletePost(id, version string, createDeletePost bool) (*Post, error) {
	post := &Post{}
	return post, client.Request(func(server *MetaPostServer) error {
		url := server.URLs.PostURL(client.Entity, id, version)
		header := make(http.Header)
		if !createDeletePost {
			header.Set("Create-Delete-Post", "false")
		}

		req, err := client.NewRequest("DELETE", url, header, nil)
		if err != nil {
			return err
		}
		res, err := HTTP.Do(req)
		if err != nil {
			return newRequestError(err, req)
		}
		return parsePostRes(post, res)
	})
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

func (client *Client) NewRequest(method, url string, header http.Header, body []byte) (*http.Request, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := NewRequest(method, url, header, bodyReader)
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
		header, err = client.requestJSONURL(method, url(server), reqHeader, body, data)
		return err
	})
}

func (client *Client) requestJSONURL(method string, url string, header http.Header, body []byte, data interface{}) (http.Header, error) {
	req, err := client.NewRequest(method, url, header, body)
	if err != nil {
		return nil, err
	}
	res, err := HTTP.Do(req)
	if err != nil {
		return nil, newRequestError(err, req)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, newResponseError(ErrBadStatusCode, res)
	}
	if ok := timeoutRead(res.Body, func() {
		err = json.NewDecoder(res.Body).Decode(data)
	}); !ok {
		return nil, newResponseError(ErrReadTimeout, res)
	}
	return res.Header, err
}

type urlFunc func(server *MetaPostServer) string

func (client *Client) requestCount(urlFunc urlFunc, header http.Header) (PageHeader, error) {
	h := PageHeader{}
	err := client.Request(func(server *MetaPostServer) error {
		req, err := client.NewRequest("HEAD", urlFunc(server), header, nil)
		if err != nil {
			return err
		}
		res, err := HTTP.Do(req)
		if err != nil {
			return newRequestError(err, req)
		}
		res.Body.Close()
		if res.StatusCode == 304 {
			h.ETag = res.Header.Get("Etag")
			h.NotModified = true
			return nil
		}
		if res.StatusCode != 200 {
			return newResponseError(ErrBadStatusCode, res)
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

func NewRequest(method, url string, header http.Header, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	if req.URL.Path != "" {
		// url.Parse unescapes the Path, which can screw with some of the signing stuff
		req.URL.Opaque = "/" + strings.SplitN(url[8:], "/", 2)[1]
		req.URL.Path = ""
		req.URL.RawQuery = ""
	}
	if header != nil {
		req.Header = header
	}
	req.Header.Set("User-Agent", UserAgent)
	return req, nil
}

type RequestError struct {
	Err     error
	Request *http.Request
}

func newRequestError(err error, req *http.Request) *RequestError {
	return &RequestError{Err: err, Request: req}
}

func (e *RequestError) Error() string {
	if e.Request != nil {
		return fmt.Sprintf("tent: error requesting %s %s: %s", e.Request.Method, e.Request.URL, e.Err.Error())
	} else {
		return "tent: request error: " + e.Err.Error()
	}
}

type ResponseErrorType int

const (
	ErrBadStatusCode ResponseErrorType = iota
	ErrBadContentType
	ErrBadData
	ErrReadTimeout
)

type ResponseError struct {
	Type      ResponseErrorType
	Response  *http.Response
	TentError *TentError
}

type TentError struct {
	Error  string            `json:"error"`
	Fields map[string]string `json:"fields"`
}

const MediaTypeError = "application/vnd.tent.error.v0+json"

func newResponseError(typ ResponseErrorType, res *http.Response) *ResponseError {
	err := &ResponseError{Type: typ, Response: res}

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

func (e *ResponseError) Error() string {
	switch e.Type {
	case ErrBadContentType:
		return fmt.Sprintf("tent: incorrect Content-Type received: %q", e.Response.Header.Get("Content-Type"))
	case ErrBadData:
		if e.Response != nil {
			return fmt.Sprintf("tent: bad post data returned from %s %s", e.Response.Request.Method, e.Response.Request.URL)
		} else {
			return "tent: bad post data"
		}
	case ErrReadTimeout:
		return fmt.Sprintf("tent: timeout reading response body of %s %s", e.Response.Request.Method, e.Response.Request.URL)
	default:
		msg := fmt.Sprintf("tent: unexpected %d performing %s %s", e.Response.StatusCode, e.Response.Request.Method, e.Response.Request.URL)
		if e.TentError != nil {
			msg += " - " + e.TentError.Error
		}
		return msg
	}
}
