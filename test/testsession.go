package main

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"strconv"
	"time"

	"github.com/nknorg/ncp-go/test/mockconn"

	ncp "github.com/nknorg/ncp-go"
)

type TestSession struct {
	localSess   *ncp.Session
	remoteSess  *ncp.Session
	mockConfigs map[string]*stMockConfig // map clinetId to mockConfig
	numClients  int

	localConns  map[string]net.Conn
	remoteConns map[string]net.Conn
}

func (ts *TestSession) Create(confs map[string]*stMockConfig, numClients int) {
	ts.mockConfigs = confs

	ts.localConns = make(map[string]net.Conn)
	ts.remoteConns = make(map[string]net.Conn)

	clientIDs := make([]string, 0)
	for i := 0; i < numClients; i++ {
		clientId := strconv.Itoa(i)
		clientIDs = append(clientIDs, clientId)
		conf := confs[clientId]
		localConn, remoteConn, err := mockconn.NewMockConn("Alice_"+clientId, "Bob_"+clientId,
			conf.throughput, conf.bufferSize, conf.latency)
		if err == nil {
			ts.localConns[clientId] = localConn
			ts.remoteConns[clientId] = remoteConn
		} else {
			log.Fatalln("mockconn.NewMockConn err:%v", err)
		}
	}

	sessionConfig := &ncp.Config{}

	localSess, _ := ncp.NewSession(mockconn.NewClientAddr("Alice"), mockconn.NewClientAddr("Bob"), clientIDs, clientIDs,
		func(localClientID, remoteClientID string, buf []byte, writeTimeout time.Duration) (err error) {
			netconn, ok := ts.localConns[localClientID]
			if ok {
				_, err = netconn.Write(buf)
			} else {
				err = errors.New("Sendwith can't get connection")
			}
			return err
		}, sessionConfig)

	remoteSess, _ := ncp.NewSession(mockconn.NewClientAddr("Bob"), mockconn.NewClientAddr("Alice"), clientIDs, clientIDs,
		func(localClientID, remoteClientID string, buf []byte, writeTimeout time.Duration) (err error) {
			conn, ok := ts.remoteConns[localClientID]
			if ok {
				_, err = conn.Write(buf)
			} else {
				err = errors.New("Sendwith can't get connection")
			}
			return err
		}, sessionConfig)

	ts.localSess = localSess
	ts.remoteSess = remoteSess

	go func() {
		for clientId, conn := range ts.localConns {
			go ts.networkRead(ts.localSess, conn, clientId)
		}
		for clientId, conn := range ts.remoteConns {
			go ts.networkRead(ts.remoteSess, conn, clientId)
		}
	}()

}

func (ts *TestSession) networkRead(s *ncp.Session, conn net.Conn, clientId string) {
	for {
		b := make([]byte, 1500)
		n, err := conn.Read(b)
		if err != nil {
			log.Fatalf("%v read error: %v\n", conn.LocalAddr().String(), err)
			time.Sleep(10 * time.Millisecond)
			continue
		}
		err = s.ReceiveWith(clientId, clientId, b[:n])
		if err != nil {
			fmt.Printf("%v ts.remoteSess.ReceiveWith error: %v\n", conn.LocalAddr().String(), err)
		}
	}
}

func (ts *TestSession) DialUp() {

	go func() {
		for {
			time.Sleep(200 * time.Millisecond)
			err := ts.remoteSess.Accept()
			if err == nil {
				break
			}
		}
	}()

	ctx := context.Background()
	err := ts.localSess.Dial(ctx)
	if err != nil {
		fmt.Printf("ts.localSess.Dial error: %v\n", err)
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
