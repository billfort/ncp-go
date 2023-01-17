package test

import (
	"fmt"
	"testing"
	"time"

	// ncp "github.com/nknorg/ncp-go"
	"github.com/nknorg/ncp-go/test/mockconn"
)

type TestCase struct {
	id                 int
	name               string
	numClients         int
	bytesToSend        int
	mockConfigs        map[string]*mockconn.ConnConfig // map localClientID to mock config
	newVersionDuration int64
	newVersionSpeed    float64
	oldVersionDuration int64
	oldVersionSpeed    float64
}

const (
	lowThroughput  = 128  // packets / second
	midThroughput  = 1024 // packets / second
	highThroughput = 2048 // packets / second

	lowLatency  = 50 * time.Millisecond  // ms
	highLatency = 500 * time.Millisecond //ms
	lowLoss     = 0.01                   // 1% loss
	highLoss    = 0.1                    // 10% loss

	bytesToSend = 8 << 20 // 8MB
)

var baseConf = mockconn.ConnConfig{Addr1: "Alice", Addr2: "Bob", Throughput: midThroughput, Latency: lowLatency}

var lowTpConf = mockconn.ConnConfig{Addr1: "Alice", Addr2: "Bob", Throughput: lowThroughput, Latency: lowLatency}
var midTpConf = baseConf
var highTpConf = mockconn.ConnConfig{Addr1: "Alice", Addr2: "Bob", Throughput: highThroughput, Latency: lowLatency}

var lowLatConf = baseConf
var highLatConf = mockconn.ConnConfig{Addr1: "Alice", Addr2: "Bob", Throughput: midThroughput, Latency: highLatency}

var lowTpHighLatConf = mockconn.ConnConfig{Addr1: "Alice", Addr2: "Bob", Throughput: lowThroughput, Latency: highLatency}

var lowLossConf = mockconn.ConnConfig{Addr1: "Alice", Addr2: "Bob", Throughput: midThroughput, Loss: lowLoss, Latency: highLatency}
var highLossConf = mockconn.ConnConfig{Addr1: "Alice", Addr2: "Bob", Throughput: midThroughput, Loss: highLoss, Latency: highLatency}

var testResult []*TestCase
var id = 0

// go test -v -run=TestBaseClient
func TestBaseClient(t *testing.T) {
	if testResult == nil {
		testResult = make([]*TestCase, 0)
	}

	id++
	tc := &TestCase{id: id, name: fmt.Sprintf("Base case, one client throughput %v packets/s, latency %v, loss %v",
		baseConf.Throughput, baseConf.Latency, baseConf.Loss), numClients: 1, bytesToSend: bytesToSend}
	tc.mockConfigs = make(map[string]*mockconn.ConnConfig)
	tc.mockConfigs["0"] = &baseConf

	// ncp.NewVersion = false // test previous version
	// tc = run(tc)
	// ncp.NewVersion = true // test new version
	tc = run(tc)

	testResult = append(testResult, tc)
	PrintResult(testResult)
}

// go test -v -run=TestBaseAndLowTpHighLat
func TestBaseAndLowTpHighLat(t *testing.T) {
	if testResult == nil {
		testResult = make([]*TestCase, 0)
	}

	id++
	tc := &TestCase{id: id, name: fmt.Sprintf("Append 1 client throughput %v packets/s, latency %v, loss %v to base case",
		lowTpHighLatConf.Throughput, lowTpHighLatConf.Latency, lowTpHighLatConf.Loss), numClients: 2, bytesToSend: bytesToSend}
	tc.mockConfigs = make(map[string]*mockconn.ConnConfig)
	tc.mockConfigs["0"] = &baseConf
	tc.mockConfigs["1"] = &lowTpHighLatConf

	// ncp.NewVersion = false // test previous version
	// tc = run(tc)
	// ncp.NewVersion = true // test new version
	tc = run(tc)
	testResult = append(testResult, tc)

	PrintResult(testResult)
}

// go test -v -run=TestBaseAnd2LowTpHighLat
func TestBaseAnd2LowTpHighLat(t *testing.T) {
	if testResult == nil {
		testResult = make([]*TestCase, 0)
	}

	id++
	tc := &TestCase{id: id, name: fmt.Sprintf("Append 2 clients throughput %v packets/s, latency %v, loss %v to base case",
		lowTpHighLatConf.Throughput, lowTpHighLatConf.Latency, lowTpHighLatConf.Loss), numClients: 3, bytesToSend: bytesToSend}
	tc.mockConfigs = make(map[string]*mockconn.ConnConfig)
	tc.mockConfigs["0"] = &baseConf
	tc.mockConfigs["1"] = &lowTpHighLatConf
	tc.mockConfigs["2"] = &highLatConf

	// ncp.NewVersion = false // test previous version
	// tc = run(tc)
	// ncp.NewVersion = true // test new version
	tc = run(tc)
	testResult = append(testResult, tc)

	PrintResult(testResult)
}

// go test -v -run=TestBaseAnd3LowTpHighLat
func TestBaseAnd3LowTpHighLat(t *testing.T) {
	if testResult == nil {
		testResult = make([]*TestCase, 0)
	}

	id++
	tc := &TestCase{id: id, name: fmt.Sprintf("Append 3 clients throughput %v packets/s, latency %v, loss %v to base case",
		lowTpHighLatConf.Throughput, lowTpHighLatConf.Latency, lowTpHighLatConf.Loss), numClients: 4, bytesToSend: bytesToSend}
	tc.mockConfigs = make(map[string]*mockconn.ConnConfig)
	tc.mockConfigs["0"] = &baseConf
	tc.mockConfigs["1"] = &lowTpHighLatConf
	tc.mockConfigs["2"] = &highLatConf
	tc.mockConfigs["3"] = &lowTpHighLatConf

	// ncp.NewVersion = false // test previous version
	// tc = run(tc)
	// ncp.NewVersion = true // test new version
	tc = run(tc)
	testResult = append(testResult, tc)

	PrintResult(testResult)
}

// go test -v -run=TestBaseAnd3LowTpHighLat
func TestBaseAnd4LowTpHighLat(t *testing.T) {
	if testResult == nil {
		testResult = make([]*TestCase, 0)
	}

	id++
	tc := &TestCase{id: id, name: fmt.Sprintf("Append 4 clients throughput %v packets/s, latency %v, loss %v to base case",
		lowTpHighLatConf.Throughput, lowTpHighLatConf.Latency, lowTpHighLatConf.Loss), numClients: 5, bytesToSend: bytesToSend}
	tc.mockConfigs = make(map[string]*mockconn.ConnConfig)
	tc.mockConfigs["0"] = &baseConf
	tc.mockConfigs["1"] = &lowTpHighLatConf
	tc.mockConfigs["2"] = &highLatConf
	tc.mockConfigs["3"] = &lowTpHighLatConf
	tc.mockConfigs["4"] = &highLatConf

	// ncp.NewVersion = false // test previous version
	// tc = run(tc)
	// ncp.NewVersion = true // test new version
	tc = run(tc)
	testResult = append(testResult, tc)

	PrintResult(testResult)
}

func run(tc *TestCase) *TestCase {
	version := "new version"
	// if !ncp.NewVersion {
	// 	version = "prev version"
	// }
	fmt.Printf("\n>>>>>> Case %v, %v, %v\n", id, version, tc.name)

	var testSess TestSession
	testSess.Create(tc.mockConfigs, tc.numClients)

	testSess.DialUp()

	writeChan := make(chan int64, 3)
	readChan := make(chan int64, 3)

	go testSess.write(testSess.localSess, tc.bytesToSend, writeChan)
	go testSess.read(testSess.remoteSess, readChan)

	bytesReceived := <-readChan
	count := <-readChan
	timeStart := <-readChan
	timeEnd := <-readChan
	duration := timeEnd - timeStart

	speed := float64(bytesReceived) / (1 << 20) / (float64(duration) / 1000.0)
	throughput := float64(count) / (float64(duration) / 1000.0)

	fmt.Printf("\n%v received %v bytes at %.3f MB/s, throughput:%.1f packets/s, duration: %v ms \n",
		testSess.remoteSess.LocalAddr(), bytesReceived, speed, throughput, duration)

	testSess.remoteSess.PrintStatic()

	// if ncp.NewVersion {
	tc.newVersionDuration = duration
	tc.newVersionSpeed = speed
	// } else {
	// 	tc.oldVersionDuration = duration
	// 	tc.oldVersionSpeed = speed
	// }

	<-writeChan // bytesSent
	timeStart = <-writeChan
	timeEnd = <-writeChan

	testSess.localSess.PrintStatic()

	return tc
}

// Print test result to compare previous version and new version
func PrintResult(testResult []*TestCase) {
	fmt.Printf("\nid \t clients \t prev speed \t new speed \t prev duration \t new duration \t test case description\n")
	for _, tc := range testResult {
		fmt.Printf("%v \t %v \t\t %.3f MB/s \t %.3f MB/s \t %v ms \t %v ms \t %v \n",
			tc.id, tc.numClients, tc.oldVersionSpeed, tc.newVersionSpeed, tc.oldVersionDuration, tc.newVersionDuration, tc.name)
	}
	fmt.Println()
}
