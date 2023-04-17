package governor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"xorkevin.dev/governor/util/kjson"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

type (
	HTTPClient interface {
		Req(method, path string, body io.Reader) (*http.Request, error)
		Do(ctx context.Context, r *http.Request) (*http.Response, error)
	}

	HTTPReqDoer interface {
		Do(r *http.Request) (*http.Response, error)
	}

	httpClient struct {
		httpc *http.Client
		base  string
	}

	configHTTPClient struct {
		baseurl   string
		timeout   time.Duration
		transport http.RoundTripper
	}
)

// Client http errors
var (
	// ErrInvalidClientReq is returned when failing to construct the client request
	ErrInvalidClientReq errInvalidClientReq
	// ErrSendClientReq is returned when failing to send the client request
	ErrSendClientReq errSendClientReq
	// ErrInvalidServerRes is returned on an invalid server response
	ErrInvalidServerRes errInvalidServerRes
	// ErrServerRes is a returned server error
	ErrServerRes errServerRes
)

type (
	errInvalidClientReq struct{}
	errSendClientReq    struct{}
	errInvalidServerRes struct{}
	errServerRes        struct{}
)

func (e errInvalidClientReq) Error() string {
	return "Invalid client request"
}

func (e errSendClientReq) Error() string {
	return "Failed sending client request"
}

func (e errInvalidServerRes) Error() string {
	return "Invalid server response"
}

func (e errServerRes) Error() string {
	return "Error server response"
}

func newHTTPClient(c configHTTPClient, l klog.Logger) *httpClient {
	return &httpClient{
		httpc: &http.Client{
			Transport: c.transport,
			Timeout:   c.timeout,
		},
		base: c.baseurl,
	}
}

func (c *httpClient) subclient(path string, l klog.Logger) HTTPClient {
	return &httpClient{
		httpc: c.httpc,
		base:  c.base + path,
	}
}

// Req creates a new request
func (c *httpClient) Req(method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, c.base+path, body)
	if err != nil {
		return nil, kerrors.WithKind(err, ErrInvalidClientReq, "Malformed request")
	}
	return req, nil
}

// Do sends a request to the server and returns its response
func (c *httpClient) Do(ctx context.Context, r *http.Request) (_ *http.Response, retErr error) {
	ctx = klog.CtxWithAttrs(ctx, klog.AString("gov.httpc.url", r.URL.String()))
	res, err := c.httpc.Do(r)
	if err != nil {
		return nil, kerrors.WithKind(err, ErrSendClientReq, "Failed request")
	}
	if res.StatusCode >= http.StatusBadRequest {
		defer func() {
			if err := res.Body.Close(); err != nil {
				retErr = errors.Join(retErr, kerrors.WithMsg(err, "Failed to close http response body"))
			}
		}()
		defer func() {
			if _, err := io.Copy(io.Discard, res.Body); err != nil {
				retErr = errors.Join(retErr, kerrors.WithMsg(err, "Failed to discard http response body"))
			}
		}()
		var errres ErrorRes
		if err := json.NewDecoder(res.Body).Decode(&errres); err != nil {
			return res, kerrors.WithKind(err, ErrInvalidServerRes, "Failed decoding response")
		}
		return res, kerrors.WithKind(nil, ErrServerRes, errres.Message)
	}
	return res, nil
}

type (
	// HTTPFetcher provides convenience HTTP client functionality
	HTTPFetcher struct {
		HTTPClient HTTPClient
	}
)

// NewHTTPFetcher creates a new [*HTTPFetcher]
func NewHTTPFetcher(c HTTPClient) *HTTPFetcher {
	return &HTTPFetcher{
		HTTPClient: c,
	}
}

// Req calls [HTTPClient] Req
func (c *HTTPFetcher) Req(method, path string, body io.Reader) (*http.Request, error) {
	return c.HTTPClient.Req(method, path, body)
}

// Do calls [HTTPClient] Do
func (c *HTTPFetcher) Do(ctx context.Context, r *http.Request) (*http.Response, error) {
	return c.HTTPClient.Do(ctx, r)
}

// ReqJSON creates a new json request
func (c *HTTPFetcher) ReqJSON(method, path string, data interface{}) (*http.Request, error) {
	b, err := kjson.Marshal(data)
	if err != nil {
		return nil, kerrors.WithKind(err, ErrInvalidClientReq, "Failed to encode body to json")
	}
	body := bytes.NewReader(b)
	req, err := c.Req(method, path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set(headerContentType, "application/json")
	return req, nil
}

// DoNoContent sends a request to the server and discards the response body
func (c *HTTPFetcher) DoNoContent(ctx context.Context, r *http.Request) (_ *http.Response, retErr error) {
	ctx = klog.CtxWithAttrs(ctx, klog.AString("gov.httpc.url", r.URL.String()))
	res, err := c.Do(ctx, r)
	if err != nil {
		return res, err
	}
	defer func() {
		if err := res.Body.Close(); err != nil {
			retErr = errors.Join(retErr, kerrors.WithMsg(err, "Failed to close http response body"))
		}
	}()
	defer func() {
		if _, err := io.Copy(io.Discard, res.Body); err != nil {
			retErr = errors.Join(retErr, kerrors.WithMsg(err, "Failed to discard http response body"))
		}
	}()
	return res, nil
}

// DoJSON sends a request to the server and decodes response json
func (c *HTTPFetcher) DoJSON(ctx context.Context, r *http.Request, response interface{}) (_ *http.Response, _ bool, retErr error) {
	ctx = klog.CtxWithAttrs(ctx, klog.AString("gov.httpc.url", r.URL.String()))
	res, err := c.Do(ctx, r)
	if err != nil {
		return res, false, err
	}
	defer func() {
		if err := res.Body.Close(); err != nil {
			retErr = errors.Join(retErr, kerrors.WithMsg(err, "Failed to close http response body"))
		}
	}()
	defer func() {
		if _, err := io.Copy(io.Discard, res.Body); err != nil {
			retErr = errors.Join(retErr, kerrors.WithMsg(err, "Failed to discard http response body"))
		}
	}()

	decoded := false
	if response != nil && isStatusDecodable(res.StatusCode) {
		dec := json.NewDecoder(res.Body)
		if err := dec.Decode(response); err != nil {
			return res, false, kerrors.WithKind(err, ErrInvalidServerRes, "Failed decoding response")
		}
		if dec.More() {
			return res, false, kerrors.WithKind(err, ErrInvalidServerRes, "Failed decoding response")
		}
		decoded = true
	}
	return res, decoded, nil
}

func isStatusDecodable(status int) bool {
	return status >= http.StatusOK && status < http.StatusMultipleChoices && status != http.StatusNoContent
}
