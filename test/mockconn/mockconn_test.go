package mockconn

import (
	"encoding/binary"
	"log"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func WriteAndRead(writer net.Conn, reader net.Conn, nPackets int, latency time.Duration) (sendSeq []int64, recvSeq []int64) {

	ch := make(chan struct{}, 2)

	go func() { // write routine

		seq := int64(1)
		count := int64(0)
		t1 := time.Now().UnixMilli()
		for i := 0; i < nPackets; i++ {
			b := make([]byte, 1024)
			binary.PutVarint(b, seq)
			_, err := writer.Write(b)
			if err != nil {
				break
			}
			sendSeq = append(sendSeq, seq)
			seq++
			count++
		}
		t2 := time.Now().UnixMilli()
		throughput := float64(count*1000) / float64(t2-t1)

		log.Printf("%v send %v packets in %v ms, throughput is: %.1f packets/s \n", writer.LocalAddr(), count, t2-t1, throughput)
		ch <- struct{}{}
	}()

	go func() { // reader routine
		b := make([]byte, 1024)
		count := int64(0)
		t1 := time.Now().UnixMilli()
		for i := 0; i < nPackets; i++ {
			_, err := reader.Read(b)
			if err != nil {
				break
			}
			seq, _ := binary.Varint(b[:8])
			recvSeq = append(recvSeq, seq)
			count++
		}
		t2 := time.Now().UnixMilli()
		ms := latency.Milliseconds()
		throughput := float64(count*1000) / float64(t2-t1-ms)

		log.Printf("%v read %v packets in %v ms, deduct %v latency, throughput is: %.1f packets/s \n",
			reader.LocalAddr(), count, t2-t1, ms, throughput)
		nc, ok := reader.(*netConn)
		if ok {
			nc.PrintMetrics()
		}
		ch <- struct{}{}
	}()

	<-ch
	<-ch

	return
}

// go test -v -run=TestBidirection
func TestBidirection(t *testing.T) {

	throughput := uint(16)
	latency := 100 * time.Millisecond // ms
	bufferSize := uint(32)

	log.Printf("Going to test bi-direction communicating, throughput is %v, latency is %v \n", throughput, latency)

	aliceConn, bobConn, err := NewMockConn("Alice", "Bob", throughput, bufferSize, latency)
	require.NotNil(t, aliceConn)
	require.NotNil(t, bobConn)
	require.Nil(t, err)

	// left write to right
	nPackets := 100
	i := 0
	sendSeq, recvSeq := WriteAndRead(aliceConn, bobConn, nPackets, latency)
	for i = 0; i < nPackets; i++ {
		if sendSeq[i] != recvSeq[i] {
			log.Printf("%v sendSeq[%v] %v != %v recvSeq[%v] %v \n",
				aliceConn.LocalAddr(), i, sendSeq[i], bobConn.LocalAddr(), i, recvSeq[i])
		}
	}
	if i == nPackets {
		log.Printf("%v write to %v %v packets, %v receive %v packets in the same sequence\n",
			aliceConn.LocalAddr(), bobConn.LocalAddr(), nPackets, bobConn.LocalAddr(), nPackets)
	}

	// right write to left
	nPackets = 50
	sendSeq, recvSeq = WriteAndRead(bobConn, aliceConn, nPackets, latency)
	for i = 0; i < nPackets; i++ {
		if sendSeq[i] != recvSeq[i] {
			log.Printf("%v sendSeq[%v] %v != %v recvSeq[%v] %v \n",
				bobConn.LocalAddr(), i, sendSeq[i], aliceConn.LocalAddr(), i, recvSeq[i])
		}
	}
	if i == nPackets {
		log.Printf("%v write to %v %v packets, %v receive %v packets in the same sequence\n",
			bobConn.LocalAddr(), aliceConn.LocalAddr(), nPackets, aliceConn.LocalAddr(), nPackets)
	}

}

// go test -v -run=TestLowThroughput
func TestLowThroughput(t *testing.T) {
	throughput := uint(16)
	latency := 20 * time.Millisecond // ms
	bufferSize := 2 * throughput

	log.Printf("Going to test low throughput at %v packets/s, latency %v\n", throughput, latency)

	aliceConn, bobConn, err := NewMockConn("Alice", "Bob", throughput, bufferSize, latency)
	require.NotNil(t, aliceConn)
	require.NotNil(t, bobConn)
	require.Nil(t, err)

	nPackets := 256
	WriteAndRead(aliceConn, bobConn, nPackets, latency)

}

// go test -v -run=TestHighThroughput
func TestHighThroughput(t *testing.T) {
	throughput := uint(512)
	latency := 20 * time.Millisecond // ms
	bufferSize := 2 * throughput

	log.Printf("Going to test high throughput at %v packets/s, latency %v\n", throughput, latency)

	aliceConn, bobConn, err := NewMockConn("Alice", "Bob", throughput, bufferSize, latency)
	require.NotNil(t, aliceConn)
	require.NotNil(t, bobConn)
	require.Nil(t, err)

	nPackets := 1024
	WriteAndRead(aliceConn, bobConn, nPackets, latency)
}

// go test -v -run=TestHighLatency
func TestHighLatency(t *testing.T) {
	throughput := uint(128)
	latency := 500 * time.Millisecond // ms
	bufferSize := 2 * throughput

	log.Printf("Going to test throughput at %v packets/s, high latency %v\n", throughput, latency)

	aliceConn, bobConn, err := NewMockConn("Alice", "Bob", throughput, bufferSize, latency)
	require.NotNil(t, aliceConn)
	require.NotNil(t, bobConn)
	require.Nil(t, err)

	nPackets := 256
	WriteAndRead(aliceConn, bobConn, nPackets, latency)

}
