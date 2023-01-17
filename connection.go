package ncp

import (
	"container/heap"
	"context"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/nknorg/ncp-go/pb"
)

const alpha = 0.1 // exponential smoothing average parameter

type Connection struct {
	session          *Session
	localClientID    string
	remoteClientID   string
	windowSize       uint32
	sendWindowUpdate chan struct{}

	sync.RWMutex
	timeSentSeq           map[uint32]time.Time
	resentSeq             map[uint32]struct{}
	sendAckQueue          SeqHeap
	retransmissionTimeout time.Duration // RTO

	// FXB
	tp        int     // recv ack, throughput
	etp       float64 // exponential throughput
	isFastest bool    // Am I fastest connection?

	// FXB, these members are for metrics, will be removed when submitting PR
	totalBytesSend int64
	totalSeqSend   int64
	totalAcqGet    int64
	numTimeout     int64 // number of time out seq

	firstSendTime time.Time     // in ms
	lastAcqTime   time.Time     // in ms
	maxRtt        time.Duration // in ms
	minRtt        time.Duration // in ms
	avgRtt        time.Duration // in ms
}

func NewConnection(session *Session, localClientID, remoteClientID string) (*Connection, error) {
	conn := &Connection{
		session:               session,
		localClientID:         localClientID,
		remoteClientID:        remoteClientID,
		windowSize:            uint32(session.config.InitialConnectionWindowSize),
		retransmissionTimeout: time.Duration(session.config.InitialRetransmissionTimeout) * time.Millisecond,
		sendWindowUpdate:      make(chan struct{}, 1),
		timeSentSeq:           make(map[uint32]time.Time),
		resentSeq:             make(map[uint32]struct{}),
		sendAckQueue:          make(SeqHeap, 0),
	}
	heap.Init(&conn.sendAckQueue)
	return conn, nil
}

func (conn *Connection) SendWindowUsed() uint32 {
	conn.RLock()
	defer conn.RUnlock()
	return uint32(len(conn.timeSentSeq))
}

func (conn *Connection) RetransmissionTimeout() time.Duration {
	conn.RLock()
	defer conn.RUnlock()
	return conn.retransmissionTimeout
}

func (conn *Connection) SendAck(sequenceID uint32) {
	conn.Lock()
	heap.Push(&conn.sendAckQueue, sequenceID)
	conn.Unlock()
}

func (conn *Connection) SendAckQueueLen() int {
	conn.RLock()
	defer conn.RUnlock()
	return conn.sendAckQueue.Len()
}

func (conn *Connection) ReceiveAck(sequenceID uint32, isSentByMe bool) {
	conn.Lock()
	defer conn.Unlock()

	t, ok := conn.timeSentSeq[sequenceID]
	if !ok {
		return
	}

	if _, ok := conn.resentSeq[sequenceID]; !ok {

		if !NewVersion {
			conn.windowSize++
			if conn.windowSize > uint32(conn.session.config.MaxConnectionWindowSize) {
				conn.windowSize = uint32(conn.session.config.MaxConnectionWindowSize)
			}
		}
	}

	if isSentByMe {
		rtt := time.Since(t)
		if conn.minRtt == 0 {
			conn.minRtt = rtt
		} else {
			if rtt < conn.minRtt {
				conn.minRtt = rtt
			}
		}
		if rtt > conn.maxRtt {
			conn.maxRtt = rtt
		}
		if conn.avgRtt == 0 {
			conn.avgRtt = rtt
		} else {
			conn.avgRtt = (conn.avgRtt*(time.Duration(conn.totalAcqGet)) + rtt) / time.Duration(conn.totalAcqGet+1)
		}

		conn.totalAcqGet++
		conn.lastAcqTime = time.Now()
		conn.tp++

		conn.retransmissionTimeout += time.Duration(math.Tanh(float64(3*rtt-conn.retransmissionTimeout)/float64(time.Millisecond)/1000) * 100 * float64(time.Millisecond))
		if conn.retransmissionTimeout > time.Duration(conn.session.config.MaxRetransmissionTimeout)*time.Millisecond {
			conn.retransmissionTimeout = time.Duration(conn.session.config.MaxRetransmissionTimeout) * time.Millisecond
		}

	}

	delete(conn.timeSentSeq, sequenceID)
	delete(conn.resentSeq, sequenceID)

	select {
	case conn.sendWindowUpdate <- struct{}{}:
	default:
	}
}

func (conn *Connection) waitForSendWindow(ctx context.Context) error {
	for conn.SendWindowUsed() >= conn.windowSize {
		select {
		case <-conn.sendWindowUpdate:
		case <-time.After(maxWait):
			return errMaxWait
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (conn *Connection) Start() {
	go conn.tx()
	go conn.sendAck()
	go conn.checkTimeout()
}

func (conn *Connection) tx() error {
	var seq uint32
	var err error
	for {
		// FXB, only fastest connection do resending task
		if seq == 0 && conn.isFastest {
			seq, err = conn.session.getResendSeq()
			if err != nil {
				return err
			}
			if seq > 0 {
				conn.session.nResend++
			}
		}

		if seq == 0 {
			err = conn.waitForSendWindow(conn.session.context)
			if err == errMaxWait {
				continue
			}
			if err != nil {
				return err
			}

			seq, err = conn.session.getSendSeq()
			if err != nil {
				return err
			}
		}

		buf := conn.session.GetDataToSend(seq)
		if len(buf) == 0 {
			conn.Lock()
			delete(conn.timeSentSeq, seq)
			delete(conn.resentSeq, seq)
			conn.Unlock()
			seq = 0
			continue
		}

		if conn.firstSendTime.IsZero() {
			conn.firstSendTime = time.Now()
		}

		err = conn.session.sendWith(conn.localClientID, conn.remoteClientID, buf, conn.retransmissionTimeout)
		if err != nil {
			if conn.session.IsClosed() {
				return ErrSessionClosed
			}
			if err == ErrConnClosed {
				return err
			}
			// log.Println(err)
			select {
			case conn.session.resendChan <- seq:
				seq = 0
			case <-conn.session.context.Done():
				return conn.session.context.Err()
			}
			time.Sleep(time.Second)
			continue
		}

		conn.Lock()
		if _, ok := conn.timeSentSeq[seq]; !ok {
			conn.timeSentSeq[seq] = time.Now()
		}
		conn.totalBytesSend += int64(len(buf))
		conn.totalSeqSend++

		delete(conn.resentSeq, seq)
		conn.Unlock()

		seq = 0
	}
}

func (conn *Connection) sendAck() error {
	for {
		select {
		case <-time.After(time.Duration(conn.session.config.SendAckInterval) * time.Millisecond):
		case <-conn.session.context.Done():
			return conn.session.context.Err()
		}

		if conn.SendAckQueueLen() == 0 {
			continue
		}

		ackStartSeqList := make([]uint32, 0)
		ackSeqCountList := make([]uint32, 0)

		conn.Lock()

		for conn.sendAckQueue.Len() > 0 && len(ackStartSeqList) < int(conn.session.config.MaxAckSeqListSize) {
			ackStartSeq := heap.Pop(&conn.sendAckQueue).(uint32)
			ackSeqCount := uint32(1)

			for conn.sendAckQueue.Len() > 0 && conn.sendAckQueue[0] == NextSeq(ackStartSeq, int64(ackSeqCount)) {
				heap.Pop(&conn.sendAckQueue)
				ackSeqCount++
			}

			ackStartSeqList = append(ackStartSeqList, ackStartSeq)
			ackSeqCountList = append(ackSeqCountList, ackSeqCount)
		}
		conn.Unlock()

		omitCount := true
		for _, c := range ackSeqCountList {
			if c != 1 {
				omitCount = false
				break
			}
		}
		if omitCount {
			ackSeqCountList = nil
		}

		// FXB
		pack := &pb.Packet{
			AckStartSeq: ackStartSeqList,
			AckSeqCount: ackSeqCountList,
			BytesRead:   conn.session.GetBytesRead(),
		}
		if NewVersion {
			// if recvWindow used is over 80%, send NACK to sender to resend it asap.
			if float64(conn.session.RecvWindowUsed()) > float64(conn.session.recvWindowSize)*0.8 {
				if conn.session.recvWindowStartSeq > conn.session.nackSeq &&
					time.Since(conn.session.recvWindowStartSeqStart) >
						time.Duration(4*conn.session.config.SendAckInterval)*time.Millisecond {
					// pack.NackSeq = conn.session.recvWindowStartSeq
					// conn.session.nackSeq = conn.session.recvWindowStartSeq
				}
			}
		}

		buf, err := proto.Marshal(pack)
		if err != nil {
			log.Println(err)
			time.Sleep(time.Second)
			continue
		}

		err = conn.session.sendWith(conn.localClientID, conn.remoteClientID, buf, conn.retransmissionTimeout)
		if err != nil {
			if err == ErrConnClosed {
				return err
			}
			log.Println(err)
			time.Sleep(time.Second)
			continue
		}

		conn.session.updateBytesReadSentTime()
	}
}

func (conn *Connection) checkTimeout() error {
	for {
		select {
		case <-time.After(time.Duration(conn.session.config.CheckTimeoutInterval) * time.Millisecond):
		case <-conn.session.context.Done():
			return conn.session.context.Err()
		}

		threshold := time.Now().Add(-conn.retransmissionTimeout)
		conn.Lock()
		for seq, t := range conn.timeSentSeq {
			if _, ok := conn.resentSeq[seq]; ok {
				continue
			}
			if t.Before(threshold) {
				select {
				case conn.session.resendChan <- seq:
					conn.resentSeq[seq] = struct{}{}
					// Previous version
					if !NewVersion {
						conn.windowSize /= 2
						if conn.windowSize < uint32(conn.session.config.MinConnectionWindowSize) {
							conn.windowSize = uint32(conn.session.config.MinConnectionWindowSize)
						}
					}

					conn.numTimeout++
				case <-conn.session.context.Done():
					return conn.session.context.Err()
				}
			}
		}
		conn.Unlock()
	}
}

// FXB
func (conn *Connection) ResetStatic() {
	conn.firstSendTime = time.Time{}
	conn.lastAcqTime = time.Time{}
	conn.totalBytesSend = 0
	conn.totalSeqSend = 0
	conn.totalAcqGet = 0
}

func (conn *Connection) resetTp() {
	conn.tp = 0
}

// Update exponential smoothing throughput
func (conn *Connection) updateEtp() {

	floatTp := float64(conn.tp)
	if conn.etp == 0 {
		conn.etp = floatTp
	} else {
		conn.etp = alpha*floatTp + (1.0-alpha)*(conn.etp)
	}
}

// Modify window size according connection's throughput
func (conn *Connection) setWindowSize(n uint32) {
	if n == 0 {
		return
	}
	if n < uint32(conn.session.config.MinConnectionWindowSize) {
		n = uint32(conn.session.config.MinConnectionWindowSize)
	}

	conn.windowSize = n

}

func (conn *Connection) PrintStatic() {
	if conn.firstSendTime.IsZero() { // not a sender
		return
	}

	fmt.Printf("%v connection %v:\n", conn.session.localAddr, conn.localClientID)

	if conn.lastAcqTime.IsZero() {
		conn.lastAcqTime = time.Now()
	}
	totalMs := float64(conn.lastAcqTime.Sub(conn.firstSendTime).Milliseconds())

	if totalMs > 0 {
		avgBytesSend := (float64(conn.totalBytesSend) / float64(1<<20)) / (totalMs / 1000.0)
		avgSeqSend := int(math.Round(float64(conn.totalSeqSend) / (totalMs / 1000.0)))
		fmt.Printf("avgBytesSend %.3f MB/s, avgSeqSend %v seq/s, number of Timeout seq %v \n",
			avgBytesSend, avgSeqSend, conn.numTimeout)
	}

	fmt.Printf("minRtt %v ms maxRtt %v ms avgRtt %v\n", conn.minRtt, conn.maxRtt, conn.avgRtt)

}
