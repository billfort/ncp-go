package mockconn

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// go test -v -run=TestSetReadDeadline
func TestSetReadDeadline(t *testing.T) {
	conf := &ConnConfig{Addr1: "Alice", Addr2: "Bob", Throughput: uint(16), Latency: 100 * time.Millisecond}
	uc, err := NewUniConn(conf)
	require.Nil(t, err)
	require.NotNil(t, uc)

	uc.SetDeadline(time.Now().Add(time.Second))
	b := make([]byte, 1500)
	n, err := uc.Read(b)
	require.Equal(t, 0, n)
	t.Log("Read with deadline, err: ", err)

	ch := make(chan struct{})
	<-ch

}
