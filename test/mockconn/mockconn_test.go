package mockconn

import (
	"encoding/binary"
	"log"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func WriteAndRead(writer *netConn, reader *netConn, nPackets int) (sendSeq []int64, recvSeq []int64) {

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

		log.Printf("%v send %v packets in %v ms, throughput is: %.1f packets/s \n", writer.connName, count, t2-t1, throughput)
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
		throughput := float64(count*1000) / float64(t2-t1)

		log.Printf("%v read %v packets in %v ms, throughput is: %.1f packets/s \n", reader.connName, count, t2-t1, throughput)
		reader.PrintMetrics()
		ch <- struct{}{}
	}()

	<-ch
	<-ch

	return
}

// go test -v -run=TestBidirection
func TestBidirection(t *testing.T) {

	throughput := uint(16)
	latency := int64(100) // ms
	bufferSize := uint(32)

	log.Printf("Going to test bi-direction communicating, throughput is %v, latency is %v ms\n", throughput, latency)

	leftConn, rightConn, err := NewMockConn("0", "0", throughput, bufferSize, latency)
	require.NotNil(t, leftConn)
	require.NotNil(t, rightConn)
	require.Nil(t, err)

	// left write to right
	nPackets := 100
	i := 0
	sendSeq, recvSeq := WriteAndRead(leftConn, rightConn, nPackets)
	for i = 0; i < nPackets; i++ {
		if sendSeq[i] != recvSeq[i] {
			log.Printf("%v sendSeq[%v] %v != %v recvSeq[%v] %v \n",
				leftConn.connName, i, sendSeq[i], rightConn.connName, i, recvSeq[i])
		}
	}
	if i == nPackets {
		log.Printf("%v write to %v %v packets, %v receive %v packets in the same sequence\n",
			leftConn.connName, rightConn.connName, nPackets, rightConn.connName, nPackets)
	}

	// right write to left
	nPackets = 50
	sendSeq, recvSeq = WriteAndRead(rightConn, leftConn, nPackets)
	for i = 0; i < nPackets; i++ {
		if sendSeq[i] != recvSeq[i] {
			log.Printf("%v sendSeq[%v] %v != %v recvSeq[%v] %v \n",
				rightConn.connName, i, sendSeq[i], leftConn.connName, i, recvSeq[i])
		}
	}
	if i == nPackets {
		log.Printf("%v write to %v %v packets, %v receive %v packets in the same sequence\n",
			rightConn.connName, leftConn.connName, nPackets, leftConn.connName, nPackets)
	}

}

// go test -v -run=TestLowThroughput
func TestLowThroughput(t *testing.T) {
	throughput := uint(16)
	latency := int64(20) // ms
	bufferSize := 2 * throughput

	log.Printf("Going to test low throughput at %v packets/s, latency %v ms\n", throughput, latency)

	leftConn, rightConn, err := NewMockConn("0", "0", throughput, bufferSize, latency)
	require.NotNil(t, leftConn)
	require.NotNil(t, rightConn)
	require.Nil(t, err)

	nPackets := 256
	WriteAndRead(leftConn, rightConn, nPackets)

}

// go test -v -run=TestHighThroughput
func TestHighThroughput(t *testing.T) {
	throughput := uint(512)
	latency := int64(20) // ms
	bufferSize := 2 * throughput

	log.Printf("Going to test high throughput at %v packets/s, latency %v ms\n", throughput, latency)

	leftConn, rightConn, err := NewMockConn("0", "0", throughput, bufferSize, latency)
	require.NotNil(t, leftConn)
	require.NotNil(t, rightConn)
	require.Nil(t, err)

	nPackets := 1024
	WriteAndRead(leftConn, rightConn, nPackets)
}

// go test -v -run=TestHighLatency
func TestHighLatency(t *testing.T) {
	throughput := uint(128)
	latency := int64(500) // ms
	bufferSize := 2 * throughput

	log.Printf("Going to test throughput at %v packets/s, high latency %v ms\n", throughput, latency)

	leftConn, rightConn, err := NewMockConn("0", "0", throughput, bufferSize, latency)
	require.NotNil(t, leftConn)
	require.NotNil(t, rightConn)
	require.Nil(t, err)

	nPackets := 256
	WriteAndRead(leftConn, rightConn, nPackets)

}
