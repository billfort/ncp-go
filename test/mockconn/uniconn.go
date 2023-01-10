package mockconn

import (
	"context"
	"errors"
	"log"
	"math/rand"
	"sync"
	"time"
)

const ( // connection status
	ConnUnknown       = 0
	ConnEstablished   = 1
	ConnWritingClosed = 2
	ConnClosed        = 3
)

var (
	zeroTime      time.Time
	ErrClosedConn error = errors.New("Connection is closed")
	ErrNilPointer error = errors.New("Data pointer is nil")
	ErrZeroLengh  error = errors.New("Zero length data to write")
	ErrUnknown    error = errors.New("UniConn unknown error")
)

type dataWithTime struct { // to trace time consuming.
	data []byte
	t    time.Time
}

// unidirectional channel, can only send data from localAddr to remoteAddr
type UniConn struct {
	localAddr  string
	remoteAddr string

	throughput uint
	bufferSize uint
	latency    time.Duration
	loss       float32

	sendChan   chan *dataWithTime
	bufferChan chan *dataWithTime
	recvChan   chan *dataWithTime

	unreadData []byte // save unread data

	sync.Mutex
	status int

	// for metrics
	nSendPacket    int64         // number of packets sent
	nRecvPacket    int64         // number of packets received
	nLoss          int64         // number of packets are random lost
	averageLatency time.Duration // average latency of all packets

	ctx         context.Context
	ctxCancel   context.CancelFunc
	readCtx     context.Context
	readCancel  context.CancelFunc
	writeCtx    context.Context
	writeCancel context.CancelFunc
}

func NewUniConn(conf *ConnConfig) (*UniConn, error) {
	if conf.Throughput > 0 && conf.BufferSize == 0 {
		conf.BufferSize = 2 * conf.Throughput
	}
	sendChan := make(chan *dataWithTime, 0)
	bufferChan := make(chan *dataWithTime, conf.BufferSize)
	recvChan := make(chan *dataWithTime, 0)

	uc := &UniConn{throughput: conf.Throughput, bufferSize: conf.BufferSize, latency: conf.Latency, loss: conf.Loss,
		sendChan: sendChan, bufferChan: bufferChan, recvChan: recvChan, localAddr: conf.Addr1, remoteAddr: conf.Addr2}

	uc.ctx, uc.ctxCancel = context.WithCancel(context.Background())
	err := uc.SetDeadline(zeroTime)
	if err != nil {
		return nil, err
	}

	uc.Lock()
	uc.status = ConnEstablished
	uc.Unlock()

	go uc.throughputRead()
	go uc.latencyRead()

	return uc, nil
}

func (uc *UniConn) Write(b []byte) (n int, err error) {

	if len(b) == 0 {
		return 0, ErrZeroLengh
	}

	dt := &dataWithTime{data: b}

	uc.Lock()
	defer uc.Unlock()

	if uc.status != ConnEstablished {
		return 0, ErrClosedConn
	} else {
		select {
		case uc.sendChan <- dt:
			uc.nSendPacket++
		case <-uc.writeCtx.Done():
			return 0, uc.writeCtx.Err()
		}
	}

	return len(b), nil
}

func (uc *UniConn) randomLoss() bool {
	if uc.loss == 0 { // no loss stimulate
		return false
	}

	rand.Seed(time.Now().UnixNano())
	l := rand.Float32()
	if l < uc.loss {
		uc.nLoss++
		return true
	}

	return false
}

// The routine to stimulate throughput
func (uc *UniConn) throughputRead() error {

	ticker := time.NewTicker(time.Second / time.Duration(uc.throughput))
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			dt, ok := <-uc.sendChan
			if dt != nil {
				if !uc.randomLoss() {
					dt.t = time.Now()
					uc.bufferChan <- dt
				}
			} else {
				uc.Lock()
				defer uc.Unlock()
				if uc.status == ConnWritingClosed && !ok { // no more data
					close(uc.bufferChan)
					return nil // exit this routine when writing is closed
				}
			}
		case <-uc.readCtx.Done():
			return uc.readCtx.Err()
		}
	}

	return nil
}

// The routine to stimulate latency
func (uc *UniConn) latencyRead() error {

	for {
		select {
		case dt, ok := <-uc.bufferChan:
			if dt != nil {
				dur := time.Since(dt.t)
				if dur < uc.latency {
					timer := time.NewTimer(uc.latency - dur)
					select {
					case <-timer.C:
						uc.recvChan <- dt

					case <-uc.readCtx.Done():
						return uc.readCtx.Err()
					}
				} else {
					uc.recvChan <- dt
				}
			} else {
				uc.Lock()
				defer uc.Unlock()
				if uc.status == ConnWritingClosed && !ok { // no more data
					close(uc.recvChan)
					return nil // exit this routine when writing is closed
				}
			}

		case <-uc.readCtx.Done():
			return uc.readCtx.Err()
		}
	}
}

func (uc *UniConn) Read(b []byte) (n int, err error) {
	// check if there is buffered unread data
	unreadLen := len(uc.unreadData)
	if unreadLen > 0 {
		if unreadLen <= len(b) {
			copy(b, uc.unreadData)
			uc.unreadData = make([]byte, 0)
			return unreadLen, nil
		} else {
			copy(b, uc.unreadData[0:len(b)])
			uc.unreadData = uc.unreadData[len(b):]
			return len(b), nil
		}
	}

	for {
		if err := uc.readCtx.Err(); err != nil {
			return 0, err
		}

		select {
		case dt, ok := <-uc.recvChan:
			if dt != nil {
				if len(dt.data) > len(b) {
					dt.data = dt.data[0:len(b)]
					n = len(b)
					uc.unreadData = dt.data[len(b):]
				} else {
					n = len(dt.data)
				}

				copy(b, dt.data)
				uc.nRecvPacket++

				if uc.averageLatency == 0 {
					uc.averageLatency = time.Since(dt.t)
				} else {
					uc.averageLatency = time.Duration(float64(uc.averageLatency)*(float64(uc.nRecvPacket-1)/float64(uc.nRecvPacket)) +
						float64(time.Since(dt.t))/float64(uc.nRecvPacket))
				}

				return n, nil

			} else {
				if !ok {
					uc.Lock()
					defer uc.Unlock()
					if uc.status == ConnWritingClosed { // no more data to write
						uc.status = ConnClosed // not more data to read
						return 0, ErrClosedConn
					}
				}
			}

		case <-uc.readCtx.Done():
			return 0, uc.readCtx.Err()
		}
	}

	return 0, ErrUnknown
}

func (uc *UniConn) Close() error {
	uc.Lock()
	defer uc.Unlock()

	if uc.status == ConnEstablished {
		uc.status = ConnWritingClosed
		uc.writeCancel()
		close(uc.sendChan)
	}

	return nil
}

func (uc *UniConn) LocalAddr() ClientAddr {
	return ClientAddr{addr: uc.localAddr}
}

func (uc *UniConn) RemoteAddr() ClientAddr {
	return ClientAddr{addr: uc.remoteAddr}
}

func (uc *UniConn) SetDeadline(t time.Time) error {
	err := uc.SetReadDeadline(t)
	if err != nil {
		return err
	}
	err = uc.SetWriteDeadline(t)
	if err != nil {
		return err
	}
	return nil
}

func (uc *UniConn) SetReadDeadline(t time.Time) error {
	if t == zeroTime {
		uc.readCtx, uc.readCancel = context.WithCancel(uc.ctx)
	} else {
		uc.readCtx, uc.readCancel = context.WithDeadline(uc.ctx, t)
	}
	return nil
}

func (uc *UniConn) SetWriteDeadline(t time.Time) error {
	if t == zeroTime {
		uc.writeCtx, uc.writeCancel = context.WithCancel(uc.ctx)
	} else {
		uc.writeCtx, uc.writeCancel = context.WithDeadline(uc.ctx, t)
	}
	return nil
}

func (uc *UniConn) PrintMetrics() {
	log.Printf("%v to %v, %v packets are sent, %v packets are received, %v packets are lost, average latency is %v, loss rate is %.3f\n",
		uc.localAddr, uc.remoteAddr, uc.nSendPacket, uc.nRecvPacket, uc.nLoss, uc.averageLatency, float64(uc.nLoss)/float64(uc.nRecvPacket))
}
