package unix_transport

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
)

func NewWithTLS(socketPath string, tlsConfig *tls.Config) *http.Transport {
	unixTransport := &http.Transport{TLSClientConfig: tlsConfig}
	unixTransport.RegisterProtocol("unix", NewUnixRoundTripperTls(socketPath, tlsConfig))
	return unixTransport
}

func New(socketPath string) *http.Transport {
	unixTransport := &http.Transport{}
	unixTransport.RegisterProtocol("unix", NewUnixRoundTripper(socketPath))
	return unixTransport
}

type UnixRoundTripper struct {
	path      string
	useTls    bool
	tlsConfig *tls.Config
}

func NewUnixRoundTripper(path string) *UnixRoundTripper {
	return &UnixRoundTripper{path: path}
}

func NewUnixRoundTripperTls(path string, tlsConfig *tls.Config) *UnixRoundTripper {
	return &UnixRoundTripper{
		path:      path,
		useTls:    true,
		tlsConfig: tlsConfig,
	}
}

// The RoundTripper (http://golang.org/pkg/net/http/#RoundTripper) for the socket transport dials the socket
// each time a request is made.
func (roundTripper UnixRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	var conn net.Conn
	var err error

	if roundTripper.useTls {
		conn, err = tls.Dial("unix", roundTripper.path, roundTripper.tlsConfig)
		if err != nil {
			return nil, err
		}

		if tc, ok := conn.(*tls.Conn); ok {
			// Handshake here, in case DialTLS didn't. TLSNextProto below
			// depends on it for knowing the connection state.
			if err := tc.Handshake(); err != nil {
				go conn.Close()
				return nil, err
			}
		}
	} else {
		conn, err = net.Dial("unix", roundTripper.path)
		if err != nil {
			return nil, err
		}
	}

	newReq, err := roundTripper.rewriteRequest(req)
	if err != nil {
		conn.Close()
		return nil, err
	}

	if err := newReq.Write(conn); err != nil {
		conn.Close()
		return nil, err
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), newReq)
	if err != nil {
		conn.Close()
		return nil, err
	}

	// Close the underlying connection once the caller is done reading the
	// response body, mirroring the lifecycle httputil.ClientConn used to give us.
	resp.Body = &connCloseReadCloser{ReadCloser: resp.Body, conn: conn}

	return resp, nil
}

// connCloseReadCloser closes the underlying connection when the response
// body is closed, since we no longer have httputil.ClientConn managing that for us.
type connCloseReadCloser struct {
	io.ReadCloser
	conn net.Conn
}

func (c *connCloseReadCloser) Close() error {
	bodyErr := c.ReadCloser.Close()
	connErr := c.conn.Close()
	if bodyErr != nil {
		return bodyErr
	}
	return connErr
}

func (roundTripper *UnixRoundTripper) rewriteRequest(req *http.Request) (*http.Request, error) {
	requestPath := req.URL.Path

	if !strings.HasPrefix(requestPath, roundTripper.path) {
		return nil, fmt.Errorf("wrong unix socket [unix://%s]. Expected unix socket is [%s]", requestPath, roundTripper.path)
	}

	reqPath := strings.TrimPrefix(requestPath, roundTripper.path)
	newReqUrl := fmt.Sprintf("unix://%s", reqPath)

	var err error
	newURL, err := url.Parse(newReqUrl)
	if err != nil {
		return nil, err
	}

	req.URL.Path = newURL.Path
	req.URL.Host = roundTripper.path
	return req, nil
}
