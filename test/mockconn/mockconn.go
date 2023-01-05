package mockconn

import (
	"fmt"
	"time"
)

type mockConn struct {
	leftConn  *netConn
	rightConn *netConn

	throughput uint
	latency    int64
	bufferSize uint

	leftBufferChan  chan *dataWithTime
	rightBufferChan chan *dataWithTime
}

func NewMockConn(localClienId, remoteCliendId string, throughput, bufferSize uint, latency int64) (*netConn, *netConn, error) {
	leftConn, err := NewNetConn(localClienId, remoteCliendId, fmt.Sprintf("Alice_%v", localClienId))
	if err != nil {
		return nil, nil, err
	}
	rightConn, err := NewNetConn(remoteCliendId, localClienId, fmt.Sprintf("Bob_%v", remoteCliendId))
	if err != nil {
		return nil, nil, err
	}

	leftBufferChan := make(chan *dataWithTime, bufferSize)
	rightBufferChan := make(chan *dataWithTime, bufferSize)

	mc := &mockConn{leftConn: leftConn, rightConn: rightConn, leftBufferChan: leftBufferChan, rightBufferChan: rightBufferChan,
		throughput: throughput, latency: latency, bufferSize: bufferSize}

	go mc.throughputRead(mc.leftConn, mc.leftBufferChan)
	go mc.throughputRead(mc.rightConn, mc.rightBufferChan)
	go mc.latencyRead(mc.leftConn, mc.rightBufferChan)
	go mc.latencyRead(mc.rightConn, mc.leftBufferChan)

	return mc.leftConn, mc.rightConn, nil

}

func (mc *mockConn) throughputRead(conn *netConn, bufferChan chan *dataWithTime) error {
	throughputInterval := int64(1000000 / mc.throughput) // micro second for ticker
	ticker := time.NewTicker(time.Duration(throughputInterval) * time.Microsecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			b := <-conn.sendChan
			b.t = time.Now().UnixMilli()
			bufferChan <- b
		}
	}
}

func (mc *mockConn) latencyRead(reader *netConn, bufferChan chan *dataWithTime) error {

	for {
		select {
		case dt := <-bufferChan:
			now := time.Now().UnixMilli()
			dur := now - dt.t
			if dur < mc.latency {
				timer := time.NewTimer(time.Duration(mc.latency-dur) * time.Millisecond)
				select {
				case <-timer.C:
					reader.recvChan <- dt
				}
			} else {
				reader.recvChan <- dt
			}
		}
	}
}
