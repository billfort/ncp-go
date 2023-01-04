package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"math/rand"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	ncp "github.com/nknorg/ncp-go"
	"github.com/nknorg/ncp-go/pb"
)

type TestSession struct {
	sendSess    *ncp.Session
	recvSess    *ncp.Session
	mockConfigs map[string]*stMockConfig // map clinetId to mockConfig
	numClients  int

	sendToRecvChan map[string]chan []byte // map local client id to throughput chan， from sender to receiver
	recvToSendChan map[string]chan []byte // map local client id to throughput chan, from receiver to sender

	// use fifo slice to control throughput, abandoned.
	tpMu    sync.Mutex
	tpTimes map[string]*[]int64 // save the time of sending of last second
}

func (ts *TestSession) SendToChan(ch chan<- []byte, buf []byte, writeTimeout time.Duration) error {
	if writeTimeout > 0 {
		select {
		case ch <- buf:
		case <-time.After(writeTimeout):
			return fmt.Errorf("SendToChan timeout %v", writeTimeout)
		}
	} else {
		ch <- buf
	}

	return nil
}

// receiver data from channel by ticker, ticker interval is counted by the throughput
func (ts *TestSession) RecvFromChan(recvSession *ncp.Session, localClientID, remoteClientID string, ch <-chan []byte, conf *stMockConfig) error {

	tickerDuration := time.Duration(1000000 / conf.throughput) // micro second for ticker
	ticker := time.NewTicker(tickerDuration * time.Microsecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			buf := <-ch
			go func() {
				recvBuf := buf
				time.Sleep(time.Duration(conf.latency) * time.Millisecond) // latency
				recvSession.ReceiveWith(remoteClientID, localClientID, recvBuf)
			}()
		}
	}

	return nil
}

func (ts *TestSession) Create(confs map[string]*stMockConfig, numClients int) {
	ts.mockConfigs = confs
	ts.tpTimes = make(map[string]*[]int64)
	ts.sendToRecvChan = make(map[string]chan []byte, numClients)
	ts.recvToSendChan = make(map[string]chan []byte, numClients)

	clientIDs := make([]string, 0)
	for i := 0; i < numClients; i++ {
		clientId := strconv.Itoa(i)
		clientIDs = append(clientIDs, clientId)

		ch1 := make(chan []byte, 0)
		ts.sendToRecvChan[clientId] = ch1
		ch2 := make(chan []byte, 0)
		ts.recvToSendChan[clientId] = ch2
	}

	sessionConfig := &ncp.Config{}

	sendSess, _ := ncp.NewSession(NewClientAddr("Alice"), NewClientAddr("Bob"), clientIDs, clientIDs,
		func(localClientID, remoteClientID string, buf []byte, writeTimeout time.Duration) error {
			ch, ok := ts.sendToRecvChan[localClientID]
			if !ok {
				return fmt.Errorf("can't get sendToRecvChan channel of localIentID %v", localClientID)
			}
			return ts.SendToChan(ch, buf, writeTimeout)
		}, sessionConfig)

	recvSess, _ := ncp.NewSession(NewClientAddr("Bob"), NewClientAddr("Alice"), clientIDs, clientIDs,
		func(localClientID, remoteClientID string, buf []byte, writeTimeout time.Duration) error {

			ch, ok := ts.recvToSendChan[localClientID]
			if !ok {
				return fmt.Errorf("can't get recvToSendChan channel of localIentID %v", localClientID)
			}
			return ts.SendToChan(ch, buf, writeTimeout)
		}, sessionConfig)

	ts.sendSess = sendSess
	ts.recvSess = recvSess

	for _, localClientID := range clientIDs {
		ch, ok := ts.sendToRecvChan[localClientID]
		if !ok {

		}
		conf := ts.mockConfigs[localClientID]
		go ts.RecvFromChan(ts.recvSess, localClientID, localClientID, ch, conf)
	}

	for _, localClientID := range clientIDs {
		ch := ts.recvToSendChan[localClientID]
		conf := ts.mockConfigs[localClientID]
		go ts.RecvFromChan(ts.sendSess, localClientID, localClientID, ch, conf)
	}

}

func (ts *TestSession) DialUp() {

	go func() {
		for {
			time.Sleep(200 * time.Millisecond)
			err := ts.recvSess.Accept()
			if err == nil {
				break
			}
		}
	}()

	ctx := context.Background()
	err := ts.sendSess.Dial(ctx)
	if err != nil {
		fmt.Printf("ts.sendSess.Dial error: %v\n", err)
		return
	}

}

func (ts *TestSession) write(s *ncp.Session, numBytes int, writeChan chan int64) error {
	timeStart := time.Now().UnixMilli()

	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, uint32(numBytes))
	_, err := s.Write(b)
	if err != nil {
		return err
	}

	var bytesSent int64
	for i := 0; i < numBytes/1024; i++ {
		b := make([]byte, 1024)
		for j := 0; j < len(b); j++ {
			b[j] = byte(bytesSent % 256)
			bytesSent++
		}
		n, err := s.Write(b)
		if err != nil {
			return err
		}
		if n != len(b) {
			return fmt.Errorf("sent %d instead of %d bytes", n, len(b))
		}
	}

	timeEnd := time.Now().UnixMilli()

	writeChan <- bytesSent
	writeChan <- timeStart
	writeChan <- timeEnd

	return nil

}

func (ts *TestSession) read(s *ncp.Session, readChan chan int64) error {

	timeStart := time.Now().UnixMilli()

	b := make([]byte, 4)
	n := 0
	for {
		m, err := s.Read(b[n:])
		if err != nil {
			return err
		}
		n += m
		if n >= 4 {
			break
		}
	}

	numBytes := int(binary.LittleEndian.Uint32(b))
	// fmt.Printf("%v Going to read %v bytes\n", s.LocalAddr(), numBytes)

	b = make([]byte, 1024)
	var bytesReceived int64

	for {
		n, err := s.Read(b)
		if err != nil {
			log.Fatal("s.Read err ", err)
			return err
		}
		for i := 0; i < n; i++ {
			if b[i] != byte(bytesReceived%256) {
				return fmt.Errorf("byte %d should be %d, got %d", bytesReceived, bytesReceived%256, b[i])
			}
			bytesReceived++
		}

		if bytesReceived == int64(numBytes) {
			timeEnd := time.Now().UnixMilli()

			readChan <- bytesReceived
			readChan <- timeStart
			readChan <- timeEnd

			return nil
		}
	}
}

// use fifo slice to control throughput, abandoned.
func (ts *TestSession) IsOverThroughput(localClientID string) error {
	ts.tpMu.Lock()
	defer ts.tpMu.Unlock()

	conf := ts.mockConfigs[localClientID]
	tpTimes := ts.tpTimes[localClientID]
	now := time.Now().UnixMilli()

	n := len(*tpTimes)
	if n < conf.throughput {
		*tpTimes = append(*tpTimes, now)
		return nil
	}

	thresh := now - 1000 // threshold for the last 1 second

	pos := sort.Search(n, func(i int) bool {
		return (*tpTimes)[i] > thresh
	})
	if pos != n {
		*tpTimes = (*tpTimes)[pos:]
	} else {
		*tpTimes = make([]int64, 0)
	}
	ts.tpTimes[localClientID] = tpTimes

	if len(*tpTimes) >= conf.throughput {
		return fmt.Errorf("Conn %v has sent %v in last second, over throughput %v now.",
			localClientID, len(*tpTimes), conf.throughput)
	}

	*tpTimes = append(*tpTimes, now)

	return nil

}

// use fifo slice to control throughput, abandoned.
func (ts *TestSession) Create0(confs map[string]*stMockConfig, numClients int) {
	ts.mockConfigs = confs
	ts.tpTimes = make(map[string]*[]int64)

	clientIDs := make([]string, 0)
	for i := 0; i < numClients; i++ {
		clientId := strconv.Itoa(i)
		clientIDs = append(clientIDs, clientId)
		s := make([]int64, 0)
		ts.tpTimes[clientId] = &s
	}

	sessionConfig := &ncp.Config{}

	sendSess, _ := ncp.NewSession(NewClientAddr("Alice"), NewClientAddr("Bob"), clientIDs, clientIDs,
		func(localClientID, remoteClientID string, buf []byte, writeTimeout time.Duration) error {
			conf := ts.mockConfigs[localClientID]

			if err := ts.IsOverThroughput(localClientID); err != nil {
				return err
			}

			packet := &pb.Packet{}
			err := proto.Unmarshal(buf, packet)
			if err != nil {
				return err
			}

			rand.Seed(time.Now().UnixNano())
			ms := rand.Intn(1)
			time.Sleep(time.Duration(ms) * time.Millisecond) // stimulate real network time consuming

			go func() {
				droped := randDropAndLatency(conf, localClientID, remoteClientID)
				if droped {
					return
				}

				ts.recvSess.ReceiveWith(remoteClientID, localClientID, buf)
			}()

			return nil
		}, sessionConfig)

	recvSess, _ := ncp.NewSession(NewClientAddr("Bob"), NewClientAddr("Alice"), clientIDs, clientIDs,
		func(localClientID, remoteClientID string, buf []byte, writeTimeout time.Duration) error {

			rand.Seed(time.Now().UnixNano())
			ms := rand.Intn(1)
			time.Sleep(time.Duration(ms) * time.Millisecond) // stimulate real network time consuming

			go func() { // receiver
				conf := ts.mockConfigs[localClientID]
				droped := randDropAndLatency(conf, remoteClientID, localClientID)
				if droped {
					return
				}

				ts.sendSess.ReceiveWith(remoteClientID, localClientID, buf)
			}()

			return nil
		}, sessionConfig)

	ts.sendSess = sendSess
	ts.recvSess = recvSess

}
