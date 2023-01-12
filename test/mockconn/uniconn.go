package mockconn

import (
	"context"
	"errors"
	"log"
	"math/rand"
	"sort"
	"time"

	"golang.org/x/time/rate"
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

	sendCh   chan *dataWithTime
	bufferCh chan *dataWithTime
	recvCh   chan *dataWithTime

	unreadData []byte // save unread data

	// for metrics
	nSendPacket    int64         // number of packets sent
	nRecvPacket    int64         // number of packets received
	nLoss          int64         // number of packets are random lost
	averageLatency time.Duration // average latency of all packets

	// one time deadline and cancel
	readCtx     context.Context
	readCancel  context.CancelFunc
	writeCtx    context.Context
	writeCancel context.CancelFunc

	// close UniConn
	closeWriteCtx       context.Context
	closeWriteCtxCancel context.CancelFunc
	closeReadCtx        context.Context
	closeReadCtxCancel  context.CancelFunc
}

func NewUniConn(conf *ConnConfig) (*UniConn, error) {

	if conf.Throughput > 0 && conf.BufferSize == 0 {
		conf.BufferSize = uint(2 * float64(conf.Throughput) * conf.Latency.Seconds())
	}
	if conf.BufferSize < conf.Throughput {
		conf.BufferSize = conf.Throughput
	}

	uc := &UniConn{throughput: conf.Throughput, bufferSize: conf.BufferSize, latency: conf.Latency, loss: conf.Loss,
		sendCh: make(chan *dataWithTime, 0), bufferCh: make(chan *dataWithTime, conf.BufferSize),
		recvCh: make(chan *dataWithTime, 0), localAddr: conf.Addr1, remoteAddr: conf.Addr2}

	err := uc.SetDeadline(zeroTime)
	if err != nil {
		return nil, err
	}

	uc.closeWriteCtx, uc.closeWriteCtxCancel = context.WithCancel(context.Background())
	uc.closeReadCtx, uc.closeReadCtxCancel = context.WithCancel(context.Background())

	if uc.throughput <= 512 {
		go uc.throughputReadByTicker()
	} else {
		go uc.throughputReadByTimeWindow()
	}
	// go uc.throughputReadByLimiter()

	go uc.latencyRead()

	return uc, nil
}

func (uc *UniConn) Write(b []byte) (n int, err error) {

	if len(b) == 0 {
		return 0, ErrZeroLengh
	}

	dt := &dataWithTime{data: b}

	select {
	case <-uc.closeWriteCtx.Done():
		return 0, uc.closeWriteCtx.Err()
	default:
	}

	select {
	case <-uc.writeCtx.Done(): // for one time deadline or for one time cancel
		return 0, uc.writeCtx.Err()

	case <-uc.closeWriteCtx.Done(): // for close unicon writing
		close(uc.sendCh)
		return 0, uc.closeWriteCtx.Err()

	case uc.sendCh <- dt:
		uc.nSendPacket++
	}

	return len(b), nil
}

func (uc *UniConn) randomLoss() bool {
	if uc.loss > 0 {
		rand.Seed(time.Now().UnixNano())
		l := rand.Float32()
		if l < uc.loss {
			uc.nLoss++
			return true
		}
	}

	return false
}

// The routine to stimulate throughput by rate Limiter
func (uc *UniConn) throughputReadByLimiter() error {

	r := rate.NewLimiter(rate.Limit(uc.throughput), 1)
	for {
		err := r.Wait(uc.readCtx)
		if err != nil {
			return err
		}

		select {
		case <-uc.closeWriteCtx.Done():
			close(uc.bufferCh)
			return uc.closeWriteCtx.Err()

		case dt := <-uc.sendCh:
			if dt != nil {
				if !uc.randomLoss() {
					dt.t = time.Now()
					uc.bufferCh <- dt
				}
			}
		}
	}

	return nil
}

// The routine to stimulate throughput by ticker
func (uc *UniConn) throughputReadByTicker() error {

	ticker := time.NewTicker(time.Second / time.Duration(uc.throughput))
	defer ticker.Stop()

	for {
		select {
		case <-uc.closeWriteCtx.Done():
			return uc.closeWriteCtx.Err()

		case <-ticker.C:
			select {
			case <-uc.closeWriteCtx.Done():
				close(uc.bufferCh)
				return uc.closeWriteCtx.Err()

			case dt := <-uc.sendCh:
				if dt != nil {
					if !uc.randomLoss() {
						dt.t = time.Now()
						uc.bufferCh <- dt
					}
				}
			}
		}
	}

	return nil
}

// The routine to stimulate throughput by moving time window
func (uc *UniConn) throughputReadByTimeWindow() error {

	s := make([]time.Time, 0, uc.throughput)

	seg := uc.throughput / 100 // segment throughtput form time window
	if seg > 20 {
		seg = 20
	}

	for {

		n := len(s)
		if uint(n) < uc.throughput/seg {
			select {
			case <-uc.closeWriteCtx.Done():
				close(uc.bufferCh)
				return uc.closeWriteCtx.Err()

			case dt := <-uc.sendCh:
				if dt != nil {
					if !uc.randomLoss() {
						now := time.Now()
						dt.t = now
						uc.bufferCh <- dt
						s = append(s, now)
					}
				}
			}
		} else {
			t := time.Now().Add(-(time.Duration(1000 / seg)) * time.Millisecond)
			i := sort.Search(n, func(i int) bool {
				return s[i].After(t)
			})
			if i >= n {
				s = make([]time.Time, 0, uc.throughput)
			} else if i > 0 && i < n {
				s = s[i:]
			}

			if uint(len(s)) >= uc.throughput/seg {
				time.Sleep(time.Millisecond)
			}
		}
	}

	return nil
}

// The routine to stimulate latency
func (uc *UniConn) latencyRead() error {

	for {
		select {
		case <-uc.closeReadCtx.Done():
			close(uc.recvCh)
			return uc.closeReadCtx.Err()

		case dt := <-uc.bufferCh:
			if dt != nil {
				dur := time.Since(dt.t)
				if dur < uc.latency {
					timer := time.NewTimer(uc.latency - dur)
					select {
					case <-uc.closeReadCtx.Done():
						close(uc.recvCh)
						return uc.closeReadCtx.Err()

					case <-timer.C:
					}
				}
				uc.recvCh <- dt
			}

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
		case <-uc.readCtx.Done():
			return 0, uc.readCtx.Err()
		case <-uc.closeReadCtx.Done():
			return 0, uc.closeReadCtx.Err()

		case dt := <-uc.recvCh:
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

			}

		}
	}

	return 0, ErrUnknown
}

func (uc *UniConn) CloseWrite() error {

	uc.closeWriteCtxCancel()

	return nil
}

func (uc *UniConn) CloseRead() error {
	uc.closeReadCtxCancel()

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
		uc.readCtx, uc.readCancel = context.WithCancel(context.Background())
	} else {
		uc.readCtx, uc.readCancel = context.WithDeadline(context.Background(), t)
	}
	return nil
}

func (uc *UniConn) SetWriteDeadline(t time.Time) error {
	if t == zeroTime {
		uc.writeCtx, uc.writeCancel = context.WithCancel(context.Background())
	} else {
		uc.writeCtx, uc.writeCancel = context.WithDeadline(context.Background(), t)
	}
	return nil
}

func (uc *UniConn) PrintMetrics() {
	log.Printf("%v to %v, %v packets are sent, %v packets are received, %v packets are lost, average latency is %v, loss rate is %.3f\n",
		uc.localAddr, uc.remoteAddr, uc.nSendPacket, uc.nRecvPacket, uc.nLoss, uc.averageLatency, float64(uc.nLoss)/float64(uc.nRecvPacket))
}
