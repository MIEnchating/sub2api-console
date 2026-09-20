package accountworkbench

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin/loginproxy"
	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
)

// Each request uses one connection and one submission. Both HTTP versions use
// the same pinned dialer; transport errors never trigger implicit replay.
type protocolTransport struct{ dialer *loginproxy.Dialer }

func (p *protocolTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if !protocolEndpoint(request.URL.String()) {
		return nil, loginproxy.ErrDestination
	}
	connection, err := p.dialer.DialContext(request.Context(), "tcp", net.JoinHostPort(request.URL.Hostname(), "443"))
	if err != nil {
		return nil, err
	}
	closeConnection := func() { _ = connection.Close() }
	stop := context.AfterFunc(request.Context(), closeConnection)
	cleanup := func() { stop(); closeConnection() }
	deadline := time.Now().Add(35 * time.Second)
	if requested, ok := request.Context().Deadline(); ok && requested.Before(deadline) {
		deadline = requested
	}
	_ = connection.SetDeadline(deadline)
	secure := utls.UClient(connection, &utls.Config{ServerName: request.URL.Hostname(), MinVersion: utls.VersionTLS12}, utls.HelloChrome_133)
	if err := secure.HandshakeContext(request.Context()); err != nil {
		cleanup()
		return nil, err
	}
	var response *http.Response
	if secure.ConnectionState().NegotiatedProtocol == "h2" {
		transport := &http2.Transport{DisableCompression: true}
		client, connectErr := transport.NewClientConn(secure)
		if connectErr != nil {
			cleanup()
			return nil, connectErr
		}
		response, err = client.RoundTrip(request)
		original := cleanup
		cleanup = func() { _ = client.Close(); original() }
	} else {
		err = request.Write(secure)
		if err == nil {
			response, err = http.ReadResponse(bufio.NewReader(secure), request)
		}
	}
	if err != nil {
		cleanup()
		return nil, err
	}
	response.Body = &protocolBody{ReadCloser: response.Body, cleanup: cleanup}
	return response, nil
}

type protocolBody struct {
	io.ReadCloser
	once    sync.Once
	cleanup func()
}

func (b *protocolBody) Close() error { err := b.ReadCloser.Close(); b.once.Do(b.cleanup); return err }
