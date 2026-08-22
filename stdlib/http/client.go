package http

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	nethttp "net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/telemetry"
)

// DefaultTimeout is how long a client waits for a response when the script did
// not say. A request with no deadline at all is the one failure mode a script
// cannot recover from, so there is no way to ask for one.
const DefaultTimeout = 30 * time.Second

// maxResponseBody is how much of a response body is read. A script reads a body
// as a string, so it is held in memory, and an unbounded read turns a hostile
// server into an out-of-memory error.
const maxResponseBody = 32 << 20 // 32 MiB

// Client is the PHP-visible HTTP\Client. The net/http client is a named
// unexported field rather than an embedded one, so a script reaches the methods
// below and nothing else net/http exports.
type Client struct {
	client  *nethttp.Client
	base    string
	headers map[string]string
}

// NewClient is an HTTP client configured by an associative array of $timeout,
// $base_url, $follow_redirects, $user_agent, $headers and $insecure. Every key
// is optional, and `new HTTP\Client` gives a client with a 30 second timeout
// that follows redirects.
//
// $timeout is in seconds and covers the whole request. $headers are sent with
// every request the client makes, and a header set on a request replaces the
// client's. $insecure disables certificate verification, which is for a test
// server and not for a service. An unrecognised key throws.
func NewClient(ctx context.Context, options any) (*Client, error) {
	config := clientConfig{Timeout: DefaultTimeout, FollowRedirects: true}

	if model.IsCollection(options) {
		var err error
		model.RangeValues(options, func(key, value any) bool {
			switch strings.ToLower(toString(key)) {
			case "timeout":
				config.Timeout = toDuration(value)
			case "base_url", "base":
				config.Base = toString(value)
			case "follow_redirects":
				config.FollowRedirects = toBool(value)
			case "user_agent":
				config.UserAgent = toString(value)
			case "insecure":
				config.Insecure = toBool(value)
			case "headers":
				config.Headers, err = toHeaders(value)
			default:
				// An unknown key is an error rather than a value quietly
				// ignored, so a typo in a script is reported where it is
				// written instead of at the far end of a request that did not
				// carry what it was meant to.
				err = fmt.Errorf("HTTP\\Client: unknown option %q", toString(key))
			}
			return err == nil
		})
		if err != nil {
			return nil, err
		}
	}
	if config.Timeout <= 0 {
		config.Timeout = DefaultTimeout
	}

	client := &nethttp.Client{Timeout: config.Timeout}
	if !config.FollowRedirects {
		client.CheckRedirect = func(*nethttp.Request, []*nethttp.Request) error {
			return nethttp.ErrUseLastResponse
		}
	}
	if config.Insecure {
		client.Transport = &nethttp.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	headers := config.Headers
	if config.UserAgent != "" {
		if headers == nil {
			headers = map[string]string{}
		}
		headers["User-Agent"] = config.UserAgent
	}
	return &Client{client: client, base: config.Base, headers: headers}, nil
}

// clientConfig is the parsed option array.
type clientConfig struct {
	Timeout         time.Duration
	Base            string
	FollowRedirects bool
	UserAgent       string
	Insecure        bool
	Headers         map[string]string
}

// Send sends $request and returns the response. A transport failure or a
// timeout throws; an HTTP error status does not, so check $response->status().
func (c *Client) Send(ctx context.Context, request *nethttp.Request) (*Response, error) {
	if request == nil {
		return nil, fmt.Errorf("HTTP\\Client: send expects an HTTP\\Request")
	}
	response, err := c.send(ctx, request)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// Get sends a GET request to $url and returns the response.
func (c *Client) Get(ctx context.Context, url string) (*Response, error) {
	request, err := NewRequest(ctx, nethttp.MethodGet, url)
	if err != nil {
		return nil, err
	}
	return c.Send(ctx, request)
}

// Post sends a POST request to $url with $body and returns the response.
func (c *Client) Post(ctx context.Context, url string, body ...string) (*Response, error) {
	request, err := NewRequest(ctx, nethttp.MethodPost, url, body...)
	if err != nil {
		return nil, err
	}
	return c.Send(ctx, request)
}

// Parallel sends every request in $requests at once and returns the responses
// under the same keys, so the call takes as long as the slowest request rather
// than the sum. $requests is an array of HTTP\Request keyed by a name the
// script chooses, each bounded by the client's timeout.
//
// One request failing does not fail the others and does not throw: that
// response reports $response->ok() as false and $response->err() as the reason,
// so a script sees every outcome. A throw means the argument was not an array
// of requests.
func (c *Client) Parallel(ctx context.Context, requests any) (map[string]*Response, error) {
	pending, err := toRequestMap(requests)
	if err != nil {
		return nil, err
	}
	if len(pending) == 0 {
		return map[string]*Response{}, nil
	}

	span := telemetry.StartSpan(ctx, "http.parallel", telemetry.KindExternal)
	span.SetAttribute("requests", len(pending))
	defer span.End()

	results := make(map[string]*Response, len(pending))
	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)
	for name, request := range pending {
		wg.Add(1)
		go func(name string, request *nethttp.Request) {
			defer wg.Done()
			response, err := c.send(ctx, request)
			if err != nil {
				// A failure is reported on the response rather than returned,
				// so one unreachable host does not hide the results of every
				// other request in the batch.
				response = &Response{err: err.Error()}
			}
			mu.Lock()
			results[name] = response
			mu.Unlock()
		}(name, request)
	}
	wg.Wait()

	return results, nil
}

// send performs one request against the client's configuration. Send and
// Parallel share it so a batched request is configured the same as a single
// one.
func (c *Client) send(ctx context.Context, request *nethttp.Request) (*Response, error) {
	prepared, err := c.prepare(ctx, request)
	if err != nil {
		return nil, err
	}

	span := telemetry.StartSpan(ctx, "http", telemetry.KindExternal)
	span.SetAttribute("method", prepared.Method)
	span.SetAttribute("host", prepared.URL.Host)
	defer span.End()

	response, err := c.client.Do(prepared)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("HTTP\\Client: %w", err)
	}
	defer response.Body.Close()

	// The body is read here rather than handed over as a stream, because a
	// script has no way to close one: PHP's request ends and whatever it did
	// not read would leak the connection.
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBody))
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("HTTP\\Client: read body: %w", err)
	}
	span.SetAttribute("status", response.StatusCode)
	span.SetAttribute("bytes", len(body))

	return &Response{
		status: int64(response.StatusCode),
		header: response.Header,
		body:   string(body),
	}, nil
}

// prepare resolves a request against the client's base URL and default
// headers. It clones rather than mutating, so the same request value can be
// sent by two clients, and so Parallel does not write to a request another
// goroutine is reading.
func (c *Client) prepare(ctx context.Context, request *nethttp.Request) (*nethttp.Request, error) {
	prepared := request.Clone(ctx)

	if c.base != "" && !prepared.URL.IsAbs() {
		base, err := url.Parse(c.base)
		if err != nil {
			return nil, fmt.Errorf("HTTP\\Client: base_url: %w", err)
		}
		prepared.URL = base.ResolveReference(prepared.URL)
		prepared.Host = prepared.URL.Host
	}
	if !prepared.URL.IsAbs() {
		return nil, fmt.Errorf("HTTP\\Client: %q is relative and no base_url is set", prepared.URL.String())
	}

	for name, value := range c.headers {
		if prepared.Header.Get(name) == "" {
			prepared.Header.Set(name, value)
		}
	}
	return prepared, nil
}

// toRequestMap reads the argument Parallel takes. A script passes a PHP array
// of HTTP\Request keyed by name; a Go caller passes the map directly.
func toRequestMap(requests any) (map[string]*nethttp.Request, error) {
	switch value := requests.(type) {
	case nil:
		return nil, fmt.Errorf("HTTP\\Client: parallel expects an array of HTTP\\Request")
	case map[string]*nethttp.Request:
		return value, nil
	}

	if !model.IsCollection(requests) {
		return nil, fmt.Errorf("HTTP\\Client: parallel expects an array of HTTP\\Request, got %T", requests)
	}

	out := map[string]*nethttp.Request{}
	var err error
	model.RangeValues(requests, func(key, value any) bool {
		request, ok := value.(*nethttp.Request)
		if !ok {
			err = fmt.Errorf("HTTP\\Client: parallel: %q is %T, want an HTTP\\Request", toString(key), value)
			return false
		}
		out[toString(key)] = request
		return true
	})
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, errors.New("HTTP\\Client: parallel was given no requests")
	}
	return out, nil
}
