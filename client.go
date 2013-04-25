package tent

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net"
	"net/http"
	"strconv"
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
	req, err := newRequest(method, uri, bytes.NewReader(data))
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
		return &BadResponseError{ErrBadStatusCode, res}
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
		return &BadResponseError{ErrReadTimeout, res}
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

func newRequest(method, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
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
	Type     BadResponseErrorType
	Response *http.Response
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
