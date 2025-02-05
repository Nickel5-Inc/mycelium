package testutil

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1024 * 1024

// TestHTTPServer represents a test HTTP server
type TestHTTPServer struct {
	t      *testing.T
	Server *httptest.Server
}

// NewTestHTTPServer creates a new test HTTP server
func NewTestHTTPServer(t *testing.T, handler http.Handler) *TestHTTPServer {
	server := httptest.NewServer(handler)
	t.Cleanup(func() {
		server.Close()
	})
	return &TestHTTPServer{
		t:      t,
		Server: server,
	}
}

// URL returns the server URL
func (s *TestHTTPServer) URL() string {
	return s.Server.URL
}

// Client returns an HTTP client configured for the test server
func (s *TestHTTPServer) Client() *http.Client {
	return s.Server.Client()
}

// TestGRPCServer represents a test gRPC server
type TestGRPCServer struct {
	t          *testing.T
	Server     *grpc.Server
	listener   *bufconn.Listener
	clientConn *grpc.ClientConn
}

// NewTestGRPCServer creates a new test gRPC server
func NewTestGRPCServer(t *testing.T, registerServer func(*grpc.Server)) *TestGRPCServer {
	lis := bufconn.Listen(bufSize)
	server := grpc.NewServer()
	registerServer(server)

	go func() {
		if err := server.Serve(lis); err != nil {
			t.Logf("Error serving gRPC: %v", err)
		}
	}()

	t.Cleanup(func() {
		server.Stop()
		lis.Close()
	})

	return &TestGRPCServer{
		t:        t,
		Server:   server,
		listener: lis,
	}
}

// Connect creates a client connection to the test gRPC server
func (s *TestGRPCServer) Connect(ctx context.Context, opts ...grpc.DialOption) *grpc.ClientConn {
	if s.clientConn != nil {
		return s.clientConn
	}

	dialer := func(context.Context, string) (net.Conn, error) {
		return s.listener.Dial()
	}

	defaultOpts := []grpc.DialOption{
		grpc.WithContextDialer(dialer),
		grpc.WithInsecure(),
	}

	conn, err := grpc.DialContext(ctx, "bufnet", append(defaultOpts, opts...)...)
	require.NoError(s.t, err)

	s.clientConn = conn
	s.t.Cleanup(func() {
		conn.Close()
	})

	return conn
}

// TestTCPServer represents a test TCP server
type TestTCPServer struct {
	t        *testing.T
	listener net.Listener
	addr     string
}

// NewTestTCPServer creates a new test TCP server
func NewTestTCPServer(t *testing.T, handler func(net.Conn)) *TestTCPServer {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := &TestTCPServer{
		t:        t,
		listener: listener,
		addr:     listener.Addr().String(),
	}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go handler(conn)
		}
	}()

	t.Cleanup(func() {
		listener.Close()
	})

	return server
}

// Addr returns the server address
func (s *TestTCPServer) Addr() string {
	return s.addr
}

// Connect creates a connection to the test TCP server
func (s *TestTCPServer) Connect() (net.Conn, error) {
	return net.Dial("tcp", s.addr)
}

// WithTestHTTPServer runs a test with an HTTP server
func WithTestHTTPServer(t *testing.T, handler http.Handler, fn func(*TestHTTPServer)) {
	server := NewTestHTTPServer(t, handler)
	fn(server)
}

// WithTestGRPCServer runs a test with a gRPC server
func WithTestGRPCServer(t *testing.T, registerServer func(*grpc.Server), fn func(*TestGRPCServer)) {
	server := NewTestGRPCServer(t, registerServer)
	fn(server)
}

// WithTestTCPServer runs a test with a TCP server
func WithTestTCPServer(t *testing.T, handler func(net.Conn), fn func(*TestTCPServer)) {
	server := NewTestTCPServer(t, handler)
	fn(server)
}
