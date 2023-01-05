package mockconn

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"
)

var (
	zeroTime   time.Time
	maxWait    = time.Second
	errMaxWait = errors.New("max wait time reached")
)

type dataWithTime struct { // to trace time consuming.
	data []byte
	t    int64
}

// implement of net.conn interface
type netConn struct {
	connName       string // unnecessary, just for better identifying.
	localClientId  string
	remoteClientId string

	sendChan chan *dataWithTime
	recvChan chan *dataWithTime

	context      context.Context
	cancel       context.CancelFunc
	readContext  context.Context
	readCancel   context.CancelFunc
	writeContext context.Context
	writeCancel  context.CancelFunc

	// for metrics
	nPacket      int64 // number of packets received
	totalLatency int64 // total latency of all packets
}

func NewNetConn(localClientId, remoteClientId string, connName string) (*netConn, error) {
	if len(localClientId) == 0 || len(remoteClientId) == 0 {
		return nil, fmt.Errorf("Wrong params")
	}

	nc := &netConn{localClientId: localClientId, remoteClientId: remoteClientId, connName: connName}

	nc.context, nc.cancel = context.WithCancel(context.Background())
	err := nc.SetDeadline(zeroTime)
	if err != nil {
		return nil, err
	}

	nc.sendChan = make(chan *dataWithTime, 0)
	nc.recvChan = make(chan *dataWithTime, 0)

	return nc, nil
}

func (nc *netConn) Read(b []byte) (n int, err error) {
	defer func() {
		if err == context.DeadlineExceeded {
			err = errors.New("Mock conn dead line exceeded")
		}
		if err == context.Canceled {
			err = errors.New("Mock conn is closed")
		}
	}()

	for {
		if err := nc.readContext.Err(); err != nil {
			return 0, err
		}

		select {
		case dt := <-nc.recvChan:
			nc.totalLatency += time.Now().UnixMilli() - dt.t
			if len(dt.data) > len(b) { // here is simplified, trim data to read buffer b
				dt.data = dt.data[0:len(b)]
				n = len(b)
			} else {
				n = len(dt.data)
			}
			copy(b, dt.data)
			nc.nPacket++

			return n, nil
		case <-nc.readContext.Done():
			return 0, nc.readContext.Err()
		}
	}

	return 0, errors.New("netconn unknown error")
}

func (nc *netConn) Write(b []byte) (n int, err error) {
	defer func() {
		if err == context.DeadlineExceeded {
			err = errors.New("Mock conn dead line exceeded")
		}
		if err == context.Canceled {
			err = errors.New("Mock conn is closed")
		}
	}()

	n = len(b)
	if n == 0 {
		return 0, errors.New("Zero length data to write")
	}

	dataWt := &dataWithTime{data: b}
	select {
	case nc.sendChan <- dataWt:
	case <-nc.context.Done():
		return 0, nc.context.Err()
	}

	return n, nil
}

func (nc *netConn) Close() error {
	nc.readCancel()
	nc.writeCancel()

	nc.cancel()

	close(nc.sendChan)
	close(nc.recvChan)

	return nil
}

func (nc *netConn) LocalAddr() ClientAddr {
	return ClientAddr{addr: nc.localClientId}
}

func (nc *netConn) RemoteAddr() ClientAddr {
	return ClientAddr{addr: nc.remoteClientId}
}

func (nc *netConn) SetDeadline(t time.Time) error {
	err := nc.SetReadDeadline(t)
	if err != nil {
		return err
	}
	err = nc.SetWriteDeadline(t)
	if err != nil {
		return err
	}
	return nil
}

func (nc *netConn) SetReadDeadline(t time.Time) error {
	if t == zeroTime {
		nc.readContext, nc.readCancel = context.WithCancel(nc.context)
	} else {
		nc.readContext, nc.readCancel = context.WithDeadline(nc.context, t)
	}
	return nil
}

func (nc *netConn) SetWriteDeadline(t time.Time) error {
	if t == zeroTime {
		nc.writeContext, nc.writeCancel = context.WithCancel(nc.context)
	} else {
		nc.writeContext, nc.writeCancel = context.WithDeadline(nc.context, t)
	}
	return nil
}

func (nc *netConn) PrintMetrics() {
	if nc.nPacket > 0 {
		log.Printf("connection's average latency of receiving packets is %v ms\n", nc.totalLatency/nc.nPacket)
	}
}
