package mockconn

import (
	"net"
	"time"
)

type netConn struct {
	sendConn *uniConn
	recvConn *uniConn
}

// Am implement of net.Conn interface
func NewNetConn(sendConn, recvConn *uniConn) *netConn {
	nc := &netConn{sendConn: sendConn, recvConn: recvConn}
	return nc
}

func (nc *netConn) Write(b []byte) (n int, err error) {
	return nc.sendConn.Write(b)
}

func (nc *netConn) Read(b []byte) (n int, err error) {
	return nc.recvConn.Read(b)
}

func (nc *netConn) Close() error {
	nc.sendConn.Close()

	return nil
}

func (nc *netConn) LocalAddr() net.Addr {
	return ClientAddr{addr: nc.sendConn.localAddr}
}

func (nc *netConn) RemoteAddr() net.Addr {
	return ClientAddr{addr: nc.sendConn.remoteAddr}
}

func (nc *netConn) SetDeadline(t time.Time) error {
	nc.sendConn.SetDeadline(t)
	nc.recvConn.SetDeadline(t)

	return nil
}

func (nc *netConn) SetReadDeadline(t time.Time) error {
	return nc.recvConn.SetReadDeadline(t)
}

func (nc *netConn) SetWriteDeadline(t time.Time) error {
	return nc.sendConn.SetWriteDeadline(t)
}

func (nc *netConn) PrintMetrics() {
	nc.recvConn.PrintMetrics()
}
