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

const alpha = 0.1 // exponential smoothing parameter

type stSeqData struct {
	seq       uint32
	sendTime  int64
	acqTime   int64
	rtt       int64 // round trip time
	numResend int   // number of sent times
}

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
	tp           int     // recv ack, throughput
	numTp        int     // number of tp
	etp          float64 // exponential throughput
	atp          float64 // average throughput
	sendTp       int     // send out in ACK interval
	numUpdateEtp int     // number of update etp

	totalBytesSend int
	avgBytesSend   float64
	totalSeqSend   int
	avgSeqSend     int
	totalAcqGet    int
	numTimeout     int         // number of time out seq
	seqData        []stSeqData // track each seq send and ack time.

	firstSendTime int64 // in ms
	lastAcqTime   int64 // in ms
	maxRtt        int64 // in ms
	minRtt        int64 // in ms
	avgRtt        int64 // in ms
	maxRto        int64 // in ms
	minRto        int64 // in ms
	avgRto        int64 // in ms
}

func (conn *Connection) ResetStatic() {
	conn.firstSendTime = 0
	conn.lastAcqTime = 0
	conn.seqData = make([]stSeqData, 0, 1000)
	conn.totalBytesSend = 0
	conn.avgBytesSend = 0
	conn.totalSeqSend = 0
	conn.avgSeqSend = 0
	conn.totalAcqGet = 0
	conn.avgRto = 0
	conn.avgRtt = 0
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

		seqData: make([]stSeqData, 0, 1000),
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
		// Previous Version
		if PreviousVersion {
			conn.windowSize++
			if conn.windowSize > uint32(conn.session.config.MaxConnectionWindowSize) {
				conn.windowSize = uint32(conn.session.config.MaxConnectionWindowSize)
			}
		}
	}

	if isSentByMe {
		rtt := time.Since(t)
		rttMs := rtt.Milliseconds()
		if conn.minRtt == 0 {
			conn.minRtt = rttMs
		} else {
			if rttMs < conn.minRtt {
				conn.minRtt = rttMs
			}
		}
		if rttMs > conn.maxRtt {
			conn.maxRtt = rttMs
		}
		if conn.avgRtt == 0 {
			conn.avgRtt = rttMs
		} else {
			conn.avgRtt = (conn.avgRtt*(int64)(conn.totalAcqGet) + rttMs) / (int64(conn.totalAcqGet) + 1)
		}

		conn.retransmissionTimeout += time.Duration(math.Tanh(float64(3*rtt-conn.retransmissionTimeout)/float64(time.Millisecond)/1000) * 100 * float64(time.Millisecond))
		if conn.retransmissionTimeout > time.Duration(conn.session.config.MaxRetransmissionTimeout)*time.Millisecond {
			conn.retransmissionTimeout = time.Duration(conn.session.config.MaxRetransmissionTimeout) * time.Millisecond
		}

		rtoMs := conn.retransmissionTimeout.Milliseconds()
		if conn.minRto == 0 {
			conn.minRto = rtoMs
		} else {
			if rtoMs < conn.minRto {
				conn.minRto = rtoMs
			}
		}
		if rtoMs > conn.maxRto {
			conn.maxRto = rtoMs
		}
		if conn.avgRto == 0 {
			conn.avgRto = rtoMs
		} else {
			conn.avgRto = (conn.avgRto*(int64)(conn.totalAcqGet) + rtoMs) / (int64(conn.totalAcqGet) + 1)
		}

		var seqdata stSeqData
		now := time.Now().UnixMilli()
		seqdata.acqTime = now
		seqdata.sendTime = t.UnixMilli()
		seqdata.rtt = rtt.Milliseconds()
		seqdata.seq = sequenceID
		conn.seqData = append(conn.seqData, seqdata)

		conn.totalAcqGet++
		conn.lastAcqTime = now

		conn.tp++
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
		if seq == 0 {
			seq, err = conn.session.getResendSeq()
			if err != nil {
				return err
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

		if conn.firstSendTime <= 0 {
			conn.firstSendTime = time.Now().UnixMilli()
		}

		// FXB
		var d TestData
		if len(buf) > 52 {
			pack := pb.Packet{}
			proto.Unmarshal(buf, &pack)
			(&d).Dec(pack.Data)
			d.ConnSend = time.Now().UnixMilli()
			b := d.Enc()
			ReplaceTestData(pack.Data, b)
			buf, _ = proto.Marshal(&pack)
		}

		// fmt.Printf("ts.send:%v, sess.send:%v, conn.tx:%v\n", d.TestSend, d.SessSend, d.ConnSend)

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
		conn.totalBytesSend += len(buf)
		conn.totalSeqSend += 1
		conn.sendTp++

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
		var totalCount int
		for conn.sendAckQueue.Len() > 0 && len(ackStartSeqList) < int(conn.session.config.MaxAckSeqListSize) {
			ackStartSeq := heap.Pop(&conn.sendAckQueue).(uint32)
			ackSeqCount := uint32(1)
			totalCount++
			for conn.sendAckQueue.Len() > 0 && conn.sendAckQueue[0] == NextSeq(ackStartSeq, int64(ackSeqCount)) {
				heap.Pop(&conn.sendAckQueue)
				ackSeqCount++
				totalCount++
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

		buf, err := proto.Marshal(&pb.Packet{
			AckStartSeq: ackStartSeqList,
			AckSeqCount: ackSeqCountList,
			BytesRead:   conn.session.GetBytesRead(),
			NackSeq:     conn.session.recvWindowStartSeq,
		})
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
					if PreviousVersion {
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

// Modify window size according connection's throughpu
func (conn *Connection) resetTp() {
	conn.tp = 0
}
func (conn *Connection) resetSendTp() {
	conn.sendTp = 0
}
func (conn *Connection) updateEtp() {
	conn.numTp++
	floatTp := float64(conn.tp)
	if conn.etp == 0 {
		conn.etp = floatTp
	} else {
		conn.etp = alpha*floatTp + (1.0-alpha)*(conn.etp)
	}
	if conn.atp == 0 {
		conn.atp = floatTp
	} else {
		conn.atp = (conn.atp*(float64(conn.numTp-1)) + floatTp) / float64(conn.numTp)
	}

}

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
	if conn.firstSendTime == 0 { // not a sender
		return
	}

	fmt.Printf("%v connection %v:\n", conn.session.localAddr, conn.localClientID)

	if conn.lastAcqTime == 0 {
		conn.lastAcqTime = time.Now().UnixMilli()
	}
	totalMs := float64(conn.lastAcqTime - conn.firstSendTime)

	if totalMs > 0 {
		conn.avgBytesSend = (float64(conn.totalBytesSend) / float64(1<<20)) / (totalMs / 1000.0)
		conn.avgSeqSend = int(math.Round(float64(conn.totalSeqSend) / (totalMs / 1000.0)))
		fmt.Printf("avgBytesSend %.3f MB/s, avgSeqSend %v seq/s, number of Timeout seq %v \n",
			conn.avgBytesSend, conn.avgSeqSend, conn.numTimeout)
	}

	fmt.Printf("minRtt %v ms maxRtt %v ms avgRtt %v\n", conn.minRtt, conn.maxRtt, conn.avgRtt)

}
