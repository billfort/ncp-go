package mockconn

import (
	"net"
	"time"
)

// The config to mock out an connection
// a connection has two address represent two end points Addr1 and Addr2.
// here we refer them as lAddr and rAddr. They can be any string you would like.
// such as Alice, Bob or any other meaningful for your application.
type ConnConfig struct {
	Addr1      string
	Addr2      string
	Throughput uint
	BufferSize uint
	Latency    time.Duration
	Loss       float32 // 0.01 = 1%
}

// Mock network connection
// Return two net.Conn(s) which represent two endpoints of this connection.
func NewMockConn(conf *ConnConfig) (net.Conn, net.Conn, error) {
	l2r, err := NewUniConn(conf)
	if err != nil {
		return nil, nil, err
	}

	conf.Addr1, conf.Addr2 = conf.Addr2, conf.Addr1 // switch address
	r2l, err := NewUniConn(conf)
	if err != nil {
		return nil, nil, err
	}

	localConn := NewNetConn(l2r, r2l)
	remoteConn := NewNetConn(r2l, l2r)

	return localConn, remoteConn, nil

}
