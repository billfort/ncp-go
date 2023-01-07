package mockconn

import (
	"net"
	"time"
)

type mockConn struct {
	localConn  net.Conn
	remoteConn net.Conn
}

func NewMockConn(localAddr, remoteAddr string, throughput, bufferSize uint, latency time.Duration) (net.Conn, net.Conn, error) {
	l2rUniConn, err := NewUniConn(localAddr, remoteAddr, throughput, bufferSize, latency)
	if err != nil {
		return nil, nil, err
	}
	r2lUniConn, err := NewUniConn(remoteAddr, localAddr, throughput, bufferSize, latency)
	if err != nil {
		return nil, nil, err
	}

	localConn := NewNetConn(l2rUniConn, r2lUniConn)
	remoteConn := NewNetConn(r2lUniConn, l2rUniConn)

	mc := &mockConn{localConn: localConn, remoteConn: remoteConn}

	return mc.localConn, mc.remoteConn, nil

}
