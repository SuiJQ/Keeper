package network

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"keeper/internal/log"
)

func TestParseSOCKS5RequestIPv4(t *testing.T) {
	data := []byte{0x05, 0x01, 0x00, 0x01, 1, 2, 3, 4, 0x1f, 0x90}

	req, err := ParseSOCKS5Request(data)
	require.NoError(t, err)
	require.NotNil(t, req)

	assert.Equal(t, byte(0x01), req.Command)
	assert.Equal(t, byte(0x01), req.AddrType)
	assert.Equal(t, "1.2.3.4", req.Host)
	assert.Equal(t, uint16(8080), req.Port)
}

func TestParseSOCKS5RequestDomain(t *testing.T) {
	domain := []byte("example.com")
	data := make([]byte, 0, 7+len(domain))
	data = append(data, 0x05, 0x01, 0x00, 0x03)
	data = append(data, byte(len(domain)))
	data = append(data, domain...)
	data = append(data, 0x00, 0x50)

	req, err := ParseSOCKS5Request(data)
	require.NoError(t, err)
	require.NotNil(t, req)

	assert.Equal(t, "example.com", req.Host)
	assert.Equal(t, uint16(80), req.Port)
}

func TestParseSOCKS5RequestIPv6(t *testing.T) {
	ip := net.ParseIP("::1")
	require.NotNil(t, ip)

	data := []byte{0x05, 0x01, 0x00, 0x04}
	data = append(data, ip...)
	data = append(data, 0x00, 0x00)

	req, err := ParseSOCKS5Request(data)
	require.NoError(t, err)
	require.NotNil(t, req)

	assert.Equal(t, byte(0x04), req.AddrType)
	assert.Equal(t, "[0:0:0:0:0:0:0:1]", req.Host)
	assert.Equal(t, uint16(0), req.Port)
}

func TestParseSOCKS5RequestErrors(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "too short", data: []byte{0x05}},
		{name: "unsupported version", data: []byte{0x04, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0}},
		{name: "unsupported command", data: []byte{0x05, 0x02, 0x00, 0x01, 0, 0, 0, 0, 0, 0}},
		{name: "unsupported address type", data: []byte{0x05, 0x01, 0x00, 0x02, 0, 0, 0, 0, 0, 0}},
		{name: "truncated ipv4", data: []byte{0x05, 0x01, 0x00, 0x01, 1, 2, 3}},
		{name: "truncated ipv6", data: []byte{0x05, 0x01, 0x00, 0x04, 1, 2, 3}},
		{name: "truncated domain", data: []byte{0x05, 0x01, 0x00, 0x03, 5, 'a', 'b', 'c'}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseSOCKS5Request(tt.data)
			require.Error(t, err)
		})
	}
}

func TestBuildSOCKS5ConnectRequestIPv4(t *testing.T) {
	addr := &net.TCPAddr{IP: net.ParseIP("1.2.3.4").To4(), Port: 8080}

	data, err := BuildSOCKS5ConnectRequest(addr)
	require.NoError(t, err)

	req, err := ParseSOCKS5Request(data)
	require.NoError(t, err)

	assert.Equal(t, "1.2.3.4", req.Host)
	assert.Equal(t, uint16(8080), req.Port)
}

func TestBuildSOCKS5ConnectRequestIPv6(t *testing.T) {
	addr := &net.TCPAddr{IP: net.ParseIP("::1"), Port: 1080}

	data, err := BuildSOCKS5ConnectRequest(addr)
	require.NoError(t, err)

	req, err := ParseSOCKS5Request(data)
	require.NoError(t, err)

	assert.Equal(t, "[0:0:0:0:0:0:0:1]", req.Host)
	assert.Equal(t, uint16(1080), req.Port)
}

func TestBuildSOCKS5ConnectRequestHost(t *testing.T) {
	data, err := BuildSOCKS5ConnectRequestHost("example.com", 443)
	require.NoError(t, err)

	req, err := ParseSOCKS5Request(data)
	require.NoError(t, err)

	assert.Equal(t, "example.com", req.Host)
	assert.Equal(t, uint16(443), req.Port)
}

func TestBuildSOCKS5ConnectRequestErrors(t *testing.T) {
	_, err := BuildSOCKS5ConnectRequest(nil)
	require.Error(t, err)

	_, err = BuildSOCKS5ConnectRequest(&net.TCPAddr{Port: 1})
	require.Error(t, err)

	_, err = BuildSOCKS5ConnectRequestHost("", 1)
	require.Error(t, err)
}

func TestParseSOCKS5ResponseSuccess(t *testing.T) {
	data := []byte{0x05, 0x00, 0x00, 0x01, 127, 0, 0, 1, 0x1f, 0x90}

	resp, err := ParseSOCKS5Response(data)
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, byte(0x00), resp.Reply)
	assert.Equal(t, "127.0.0.1", resp.Host)
	assert.Equal(t, uint16(8080), resp.Port)
}

func TestParseSOCKS5ResponseErrors(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "too short", data: []byte{0x05}},
		{name: "unsupported version", data: []byte{0x04, 0x00, 0x00, 0x01, 127, 0, 0, 1, 0x00, 0x00}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseSOCKS5Response(tt.data)
			require.Error(t, err)
		})
	}
}

func TestBuildSOCKS5Response(t *testing.T) {
	addr := &net.TCPAddr{IP: net.ParseIP("0.0.0.0").To4(), Port: 0}

	data, err := BuildSOCKS5Response(addr, 0x00)
	require.NoError(t, err)

	resp, err := ParseSOCKS5Response(data)
	require.NoError(t, err)

	assert.Equal(t, byte(0x00), resp.Reply)
	assert.Equal(t, "0.0.0.0", resp.Host)
}

func TestBuildSOCKS5ResponseErrors(t *testing.T) {
	_, err := BuildSOCKS5Response(nil, 0x00)
	require.Error(t, err)
}

func TestSOCKS5RequestResponseRoundTrip(t *testing.T) {
	tests := []struct {
		host string
		port uint16
	}{
		{host: "1.2.3.4", port: 8080},
		{host: "example.com", port: 443},
		{host: "[::1]", port: 1080},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			var data []byte
			if ip := net.ParseIP(tt.host); ip != nil {
				addr := &net.TCPAddr{IP: ip, Port: int(tt.port)}
				var err error
				data, err = BuildSOCKS5ConnectRequest(addr)
				require.NoError(t, err)
			} else {
				var err error
				data, err = BuildSOCKS5ConnectRequestHost(tt.host, tt.port)
				require.NoError(t, err)
			}

			req, err := ParseSOCKS5Request(data)
			require.NoError(t, err)
			assert.Equal(t, tt.host, req.Host)
			assert.Equal(t, tt.port, req.Port)
		})
	}
}

func TestSOCKS5ServerEndToEndNoAuth(t *testing.T) {
	backend, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer backend.Close()

	go func() {
		for {
			conn, err := backend.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 1024)
				n, _ := c.Read(buf)
				if n > 0 {
					_, _ = c.Write([]byte("backend-ok"))
				}
			}(conn)
		}
	}()

	server := NewSOCKS5Server("127.0.0.1:0", nil, log.Global())
	err = server.Start()
	require.NoError(t, err)
	defer func() { _ = server.Stop() }()

	conn, err := net.Dial("tcp", server.Addr())
	require.NoError(t, err)
	defer conn.Close()

	_, _ = conn.Write([]byte{0x05, 0x01, 0x00})
	buf := make([]byte, 256)
	n, _ := conn.Read(buf)
	require.Equal(t, []byte{0x05, 0x00}, buf[:n])

	backendAddr := backend.Addr().(*net.TCPAddr)
	req := []byte{0x05, 0x01, 0x00, 0x01, 127, 0, 0, 1, byte(backendAddr.Port >> 8), byte(backendAddr.Port)}
	_, _ = conn.Write(req)

	_, _ = conn.Read(buf)
	require.Equal(t, byte(0x05), buf[0])
	require.Equal(t, byte(0x00), buf[1])

	_, _ = conn.Write([]byte("ping"))
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	response := make([]byte, 1024)
	n, err = conn.Read(response)
	require.NoError(t, err)
	assert.Equal(t, "backend-ok", string(response[:n]))
}

func TestSOCKS5ServerEndToEndWithAuth(t *testing.T) {
	backend, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer backend.Close()

	go func() {
		for {
			conn, err := backend.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 1024)
				n, _ := c.Read(buf)
				if n > 0 {
					_, _ = c.Write([]byte("auth-backend-ok"))
				}
			}(conn)
		}
	}()

	server := NewSOCKS5Server("127.0.0.1:0", &ProxyAuth{Username: "alice", Password: "secret"}, log.Global())
	err = server.Start()
	require.NoError(t, err)
	defer func() { _ = server.Stop() }()

	conn, err := net.Dial("tcp", server.Addr())
	require.NoError(t, err)
	defer conn.Close()

	_, _ = conn.Write([]byte{0x05, 0x01, 0x02})
	buf := make([]byte, 256)
	n, _ := conn.Read(buf)
	require.Equal(t, []byte{0x05, 0x02}, buf[:n])

	authReq := []byte{0x01, 0x05, 0x61, 0x6c, 0x69, 0x63, 0x65, 0x06, 0x73, 0x65, 0x63, 0x72, 0x65, 0x74}
	_, _ = conn.Write(authReq)
	n, _ = conn.Read(buf)
	require.Equal(t, []byte{0x01, 0x00}, buf[:n])

	backendAddr := backend.Addr().(*net.TCPAddr)
	req := []byte{0x05, 0x01, 0x00, 0x01, 127, 0, 0, 1, byte(backendAddr.Port >> 8), byte(backendAddr.Port)}
	_, _ = conn.Write(req)

	_, _ = conn.Read(buf)
	require.Equal(t, byte(0x05), buf[0])
	require.Equal(t, byte(0x00), buf[1])

	_, _ = conn.Write([]byte("ping-auth"))
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	response := make([]byte, 1024)
	n, err = conn.Read(response)
	require.NoError(t, err)
	assert.Equal(t, "auth-backend-ok", string(response[:n]))
}

func TestSOCKS5ServerEndToEndAuthFailure(t *testing.T) {
	server := NewSOCKS5Server("127.0.0.1:0", &ProxyAuth{Username: "alice", Password: "secret"}, log.Global())
	err := server.Start()
	require.NoError(t, err)
	defer func() { _ = server.Stop() }()

	conn, err := net.Dial("tcp", server.Addr())
	require.NoError(t, err)
	defer conn.Close()

	_, _ = conn.Write([]byte{0x05, 0x01, 0x02})
	buf := make([]byte, 256)
	n, _ := conn.Read(buf)
	require.Equal(t, []byte{0x05, 0x02}, buf[:n])

	_, _ = conn.Write([]byte{0x01, 0x05, 0x61, 0x6c, 0x69, 0x63, 0x65, 0x05, 0x77, 0x72, 0x6f, 0x6e, 0x67})
	n, _ = conn.Read(buf)
	require.Equal(t, []byte{0x01, 0x01}, buf[:n])
}

func TestSOCKS5ServerEndToEndUnsupportedCommand(t *testing.T) {
	server := NewSOCKS5Server("127.0.0.1:0", nil, log.Global())
	err := server.Start()
	require.NoError(t, err)
	defer func() { _ = server.Stop() }()

	conn, err := net.Dial("tcp", server.Addr())
	require.NoError(t, err)
	defer conn.Close()

	_, _ = conn.Write([]byte{0x05, 0x01, 0x00})
	buf := make([]byte, 256)
	n, _ := conn.Read(buf)
	require.Equal(t, []byte{0x05, 0x00}, buf[:n])

	_, _ = conn.Write([]byte{0x05, 0x02, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
	n, _ = conn.Read(buf)
	require.Equal(t, []byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0}, buf[:n])
}

func TestSOCKS5ServerEndToEndBadVersionIgnored(t *testing.T) {
	server := NewSOCKS5Server("127.0.0.1:0", nil, log.Global())
	err := server.Start()
	require.NoError(t, err)
	defer func() { _ = server.Stop() }()

	conn, err := net.Dial("tcp", server.Addr())
	require.NoError(t, err)
	defer conn.Close()

	_, _ = conn.Write([]byte{0x04, 0x01, 0x00})

	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	buf := make([]byte, 256)
	_, err = conn.Read(buf)
	require.Error(t, err)
}
