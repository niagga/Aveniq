package metrics_test

import (
	"net"
	"net/http"
)

// serveOnAddress makes an http.Server answer on an OS-chosen port: the runtime
// equivalent of pointing METRICS_ADDR at 127.0.0.1:0 for a test.
func serveOnAddress(server *http.Server) (net.Listener, error) {
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return nil, err
	}
	go func() { _ = server.Serve(listener) }()
	return listener, nil
}
