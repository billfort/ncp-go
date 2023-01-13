package test

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"strconv"
	"time"

	"github.com/golang/protobuf/proto"
	ncp "github.com/nknorg/ncp-go"
	"github.com/nknorg/ncp-go/pb"
	"github.com/nknorg/ncp-go/test/mockconn"
)

type TestSession struct {
	localSess   *ncp.Session
	remoteSess  *ncp.Session
	mockConfigs map[string]*mockconn.ConnConfig // map clinetId to mockConfig
	numClients  int

	localConns  map[string]net.Conn
	remoteConns map[string]net.Conn

	closeChan chan struct{} // indicate this session is closed
}

func (ts *TestSession) Create(confs map[string]*mockconn.ConnConfig, numClients int) {
	ts.mockConfigs = confs

	ts.localConns = make(map[string]net.Conn)
	ts.remoteConns = make(map[string]net.Conn)
	ts.closeChan = make(chan struct{})

	clientIDs := make([]string, 0)
	for i := 0; i < numClients; i++ {
		clientId := strconv.Itoa(i)
		clientIDs = append(clientIDs, clientId)
		conf := confs[clientId]
		localConn, remoteConn, err := mockconn.NewMockConn(conf)
		if err == nil {
			ts.localConns[clientId] = localConn
			ts.remoteConns[clientId] = remoteConn
		} else {
			log.Fatalln("mockconn.NewMockConn err:", err)
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
	count := 0
	var zeroTime time.Time
	var t1 time.Time

loop:
	for {
		b := make([]byte, 1500)
		n, err := conn.Read(b)
		if err != nil {
			break
		}
		if t1 == zeroTime {
			t1 = time.Now()
		}

		if n > 52 {
			var d TestData
			pack := pb.Packet{}
			proto.Unmarshal(b, &pack)
			(&d).Dec(pack.Data)
			d.ConnRecv = time.Now().UnixMilli()
			bb := d.Enc()
			ReplaceTestData(pack.Data, bb)
			b, _ = proto.Marshal(&pack)
		}

		err = s.ReceiveWith(clientId, clientId, b[:n])
		if err != nil {
			fmt.Printf("%v testsession.networkRead s.ReceiveWith error: %v\n", conn.LocalAddr().String(), err)
		}
		count++

		select {
		case <-ts.closeChan:
			break loop
		default:
		}
	}

	ts.closeChan <- struct{}{}
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
	t1 := time.Now().UnixMilli()

	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, uint32(numBytes))
	_, err := s.Write(b)
	if err != nil {
		return err
	}

	d := TestData{}

	var bytesSent, count int64
	for i := 0; i < numBytes/1024; i++ {
		b := make([]byte, 1024)

		d.TestSend = time.Now().UnixMilli()
		db := d.Enc()
		ReplaceTestData(b, db)

		n, err := s.Write(b)
		if err != nil {
			log.Fatal("testsession.write s.Write err ", err)
			return err
		}
		if n != len(b) {
			return fmt.Errorf("sent %d instead of %d bytes", n, len(b))
		}
		bytesSent += int64(len(b))
		count++
	}

	t2 := time.Now().UnixMilli()

	writeChan <- bytesSent
	writeChan <- count
	writeChan <- t1
	writeChan <- t2

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

	b = make([]byte, 1024)
	var bytesReceived, count int64

	for {
		n, err := s.Read(b)
		if err != nil {
			log.Fatal("testsession.read s.Read err ", err)
			return err
		}
		bytesReceived += int64(n)
		count++

		if bytesReceived == int64(numBytes) {
			timeEnd := time.Now().UnixMilli()

			readChan <- bytesReceived
			readChan <- count
			readChan <- timeStart
			readChan <- timeEnd

			return nil
		}
	}
}

func (ts *TestSession) Close() {
	for _, conn := range ts.localConns {
		conn.Close()
	}
	for _, conn := range ts.remoteConns {
		conn.Close()
	}
	ts.localSess.Close()
	ts.remoteSess.Close()

	<-ts.closeChan
	<-ts.closeChan
}
