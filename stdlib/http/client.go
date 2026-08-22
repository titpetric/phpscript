package http

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	nethttp "net/http"
	"strings"
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

// Client is the PHP-visible HTTP\Client. As with Request, the net/http client
// is a named unexported field so that a script reaches only the methods below.
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
func (c *Client) Send(ctx context.Context, request *Request) (*Response, error) {
	if request == nil {
		return nil, fmt.Errorf("HTTP\\Client: send expects an HTTP\\Request")
	}

	built, err := request.build(ctx, c.base)
	if err != nil {
		return nil, err
	}
	for name, value := range c.headers {
		if built.Header.Get(name) == "" {
			built.Header.Set(name, value)
		}
	}

	span := telemetry.StartSpan(ctx, "http", telemetry.KindExternal)
	span.SetAttribute("method", built.Method)
	span.SetAttribute("host", built.URL.Host)
	defer span.End()

	response, err := c.client.Do(built)
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
