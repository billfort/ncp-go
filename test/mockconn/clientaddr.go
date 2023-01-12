package mockconn

// ClientAddr represents MockConn client address. It implements net.Addr interface.
type ClientAddr struct {
	addr string
}

// NewClientAddr creates a ClientAddr from a client address string.
func NewClientAddr(addr string) *ClientAddr {
	return &ClientAddr{addr: addr}
}

// Network returns "nkn"
func (addr ClientAddr) Network() string { return "mockconn" }

// String returns the NKN client address string.
func (addr ClientAddr) String() string { return addr.addr }
