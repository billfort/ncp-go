package mockconn

import (
	"context"
	"errors"
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
	t    time.Time
}

// unidirectional channel, can only send data from localAddr to remoteAddr
type uniConn struct {
	localAddr  string
	remoteAddr string

	throughput uint
	bufferSize uint
	latency    time.Duration

	sendChan   chan *dataWithTime
	bufferChan chan *dataWithTime
	recvChan   chan *dataWithTime

	// for metrics
	nPacket      int64         // number of packets received
	totalLatency time.Duration // total latency of all packets

	context      context.Context
	cancel       context.CancelFunc
	readContext  context.Context
	readCancel   context.CancelFunc
	writeContext context.Context
	writeCancel  context.CancelFunc
}

func NewUniConn(localAddr, remoteAddr string, throughput, bufferSize uint, latency time.Duration) (*uniConn, error) {
	if throughput > 0 && bufferSize == 0 {
		bufferSize = 2 * throughput
	}
	sendChan := make(chan *dataWithTime, 0)
	bufferChan := make(chan *dataWithTime, bufferSize)
	recvChan := make(chan *dataWithTime, 0)

	uc := &uniConn{throughput: throughput, bufferSize: bufferSize, latency: latency,
		sendChan: sendChan, bufferChan: bufferChan, recvChan: recvChan, localAddr: localAddr, remoteAddr: remoteAddr}

	uc.context, uc.cancel = context.WithCancel(context.Background())
	err := uc.SetDeadline(zeroTime)
	if err != nil {
		return nil, err
	}

	go uc.throughputRead()
	go uc.latencyRead()

	return uc, nil
}

func (uc *uniConn) Write(b []byte) (n int, err error) {
	n = len(b)
	if n == 0 {
		return 0, errors.New("Zero length data to write")
	}

	dt := &dataWithTime{data: b}
	select {
	case uc.sendChan <- dt:
	case <-uc.writeContext.Done():
		return 0, uc.writeContext.Err()
	}

	return n, nil
}

func (uc *uniConn) throughputRead() error {

	ticker := time.NewTicker(time.Second / time.Duration(uc.throughput))
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			b := <-uc.sendChan
			b.t = time.Now()
			uc.bufferChan <- b

		case <-uc.writeContext.Done():
			return uc.writeContext.Err()
		}
	}
	return nil
}

func (uc *uniConn) latencyRead() error {

	for {
		select {
		case dt := <-uc.bufferChan:
			dur := time.Since(dt.t)
			if dur < uc.latency {
				timer := time.NewTimer(uc.latency - dur)
				select {
				case <-timer.C:
					uc.recvChan <- dt
				}
			} else {
				uc.recvChan <- dt
			}
		}
	}
}

func (uc *uniConn) Read(b []byte) (n int, err error) {

	for {
		if err := uc.readContext.Err(); err != nil {
			return 0, err
		}

		select {
		case dt := <-uc.recvChan:
			uc.totalLatency += time.Since(dt.t)
			if len(dt.data) > len(b) { // here is simplified, trim data to read buffer b
				dt.data = dt.data[0:len(b)]
				n = len(b)
			} else {
				n = len(dt.data)
			}
			copy(b, dt.data)
			uc.nPacket++

			return n, nil
		case <-uc.readContext.Done():
			return 0, uc.readContext.Err()
		}
	}

	return 0, errors.New("netconn unknown error")
}

func (uc *uniConn) Close() error {
	uc.cancel()
	uc.readCancel()
	uc.writeCancel()

	close(uc.sendChan)
	close(uc.bufferChan)
	close(uc.recvChan)

	return nil
}

func (uc *uniConn) LocalAddr() ClientAddr {
	return ClientAddr{addr: uc.localAddr}
}

func (uc *uniConn) RemoteAddr() ClientAddr {
	return ClientAddr{addr: uc.remoteAddr}
}

func (uc *uniConn) SetDeadline(t time.Time) error {
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

func (uc *uniConn) SetReadDeadline(t time.Time) error {
	if t == zeroTime {
		uc.readContext, uc.readCancel = context.WithCancel(uc.context)
	} else {
		uc.readContext, uc.readCancel = context.WithDeadline(uc.context, t)
	}
	return nil
}

func (uc *uniConn) SetWriteDeadline(t time.Time) error {
	if t == zeroTime {
		uc.writeContext, uc.writeCancel = context.WithCancel(uc.context)
	} else {
		uc.writeContext, uc.writeCancel = context.WithDeadline(uc.context, t)
	}
	return nil
}

func (uc *uniConn) PrintMetrics() {
	if uc.nPacket > 0 {
		log.Printf("%v to %v average latency is %v\n", uc.localAddr, uc.remoteAddr, uc.totalLatency/time.Duration(uc.nPacket))
	}
}
