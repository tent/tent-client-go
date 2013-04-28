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

const PostMediaType = "application/vnd.tent.post.v0+json"

func (client *Client) CreatePost(post *Post) error {
	data, err := json.Marshal(post)
	if err != nil {
		return err
	}
	var method, uri string
	if post.ID == "" || post.Version.ID == "" {
		method, uri = "POST", client.Servers[0].URLs.NewPost
	} else {
		method = "PUT"
		uri, err = client.Servers[0].URLs.PostURL(post.Entity, post.ID, post.Version.ID)
		if err != nil {
			return err
		}
	}
	req, err := newRequest(method, uri, nil, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mime.FormatMediaType(PostMediaType, map[string]string{"type": post.Type}))
	if len(post.Links) > 0 {
		req.Header.Set("Link", link.Format(post.Links))
		post.Links = nil
	}
	res, err := HTTP.Do(req)
	if err != nil {
		return err
	}
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

	if ok := timeoutRead(res.Body, func() {
		err = json.NewDecoder(res.Body).Decode(post)
	}); !ok {
		return newBadResponseError(ErrReadTimeout, res)
	}

	return err
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

func (client *Client) SignRequest(req *http.Request) {
	auth := hawk.NewRequestAuth(req, client.Credentials, 0)
	req.Header.Set("Authorization", auth.RequestHeader())
}

func (client *Client) newRequest(method, url string, header http.Header, body io.Reader) (*http.Request, error) {
	req, err := newRequest(method, url, header, body)
	if err != nil {
		return nil, err
	}
	client.SignRequest(req)
	return req, nil
}

func (client *Client) requestJSON(method string, url func(server *MetaPostServer) (string, error), header http.Header, body []byte, data interface{}) error {
	return client.Request(func(server *MetaPostServer) error {
		uri, err := url(server)
		if err != nil {
			return err
		}
		var bodyReader io.Reader
		if body != nil {
			bodyReader = bytes.NewReader(body)
		}
		req, err := client.newRequest(method, uri, header, bodyReader)
		if err != nil {
			return err
		}
		res, err := HTTP.Do(req)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			return newBadResponseError(ErrBadStatusCode, res)
		}
		if ok := timeoutRead(res.Body, func() {
			err = json.NewDecoder(res.Body).Decode(data)
		}); !ok {
			return newBadResponseError(ErrReadTimeout, res)
		}
		return err
	})
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
