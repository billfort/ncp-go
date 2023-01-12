package test

import (
	"flag"
	"fmt"
	"testing"
	"time"

	ncp "github.com/nknorg/ncp-go"
	"github.com/nknorg/ncp-go/test/mockconn"
)

type stTestCase struct {
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
	lowThroughput  = 32   // packets / second
	midThroughput  = 1024 // packets / second
	highThroughput = 2048 // packets / second

	lowLatency  = 20 * time.Millisecond  // ms
	highLatency = 300 * time.Millisecond //ms
	lowLoss     = 0.01                   // 1% loss
	highLoss    = 0.1                    // 10% loss
)

var baseConf = mockconn.ConnConfig{Addr1: "Alice", Addr2: "Bob", Throughput: midThroughput, Loss: 0, Latency: lowLatency}

var lowTpConf = mockconn.ConnConfig{Addr1: "Alice", Addr2: "Bob", Throughput: lowThroughput, Loss: 0, Latency: lowLatency}
var midTpConf = baseConf
var highTpConf = mockconn.ConnConfig{Addr1: "Alice", Addr2: "Bob", Throughput: highThroughput, Loss: 0, Latency: lowLatency}

var lowLatConf = baseConf
var highLatConf = mockconn.ConnConfig{Addr1: "Alice", Addr2: "Bob", Throughput: midThroughput, Loss: 0, Latency: highLatency}

var lowLossConf = mockconn.ConnConfig{Addr1: "Alice", Addr2: "Bob", Throughput: midThroughput, Loss: lowLoss, Latency: highLatency}
var highLossConf = mockconn.ConnConfig{Addr1: "Alice", Addr2: "Bob", Throughput: midThroughput, Loss: highLoss, Latency: highLatency}

var testResult []*stTestCase
var id = 0
var bytesToSend = 8 << 20 // 8MB

// go test -v -run=TestBaseClient
func TestBaseClient(t *testing.T) {
	if testResult == nil {
		testResult = make([]*stTestCase, 0)
	}

	id++
	tc := &stTestCase{id: id, name: fmt.Sprintf("Base case, one client throughput %v packets/s, latency %v, loss %v",
		baseConf.Throughput, baseConf.Latency, baseConf.Loss), numClients: 1, bytesToSend: bytesToSend}
	tc.mockConfigs = make(map[string]*mockconn.ConnConfig)
	tc.mockConfigs["0"] = &baseConf

	ncp.PreviousVersion = true // test previous version
	tc = run(tc)
	ncp.PreviousVersion = false // test new version
	tc = run(tc)

	testResult = append(testResult, tc)
}

// go test -v -run=TestBaseAndLowTp
func TestBaseAndLowTp(t *testing.T) {
	if testResult == nil {
		testResult = make([]*stTestCase, 0)
	}

	tc := &stTestCase{id: id, name: fmt.Sprintf("Append one client throughput %v packets/s, latency %v, loss %v to base case",
		lowTpConf.Throughput, lowTpConf.Latency, lowTpConf.Loss), numClients: 2, bytesToSend: bytesToSend}
	tc.mockConfigs = make(map[string]*mockconn.ConnConfig)
	tc.mockConfigs["0"] = &baseConf
	tc.mockConfigs["1"] = &lowTpConf

	ncp.PreviousVersion = true // test previous version
	tc = run(tc)
	ncp.PreviousVersion = false // test new version
	tc = run(tc)
	testResult = append(testResult, tc)
}

// go test -v -run=TestBaseAndLowAndHighTp
func TestBaseAndLowAndHighTp(t *testing.T) {
	if testResult == nil {
		testResult = make([]*stTestCase, 0)
	}

	tc := &stTestCase{id: id, name: fmt.Sprintf("Append 2 clients, one low %v packets/s, the other high %v packets/s to base case",
		lowTpConf.Throughput, highTpConf.Throughput), numClients: 3, bytesToSend: bytesToSend}
	tc.mockConfigs = make(map[string]*mockconn.ConnConfig)
	tc.mockConfigs["0"] = &lowTpConf
	tc.mockConfigs["1"] = &baseConf
	tc.mockConfigs["2"] = &highTpConf

	ncp.PreviousVersion = true // test previous version
	tc = run(tc)
	ncp.PreviousVersion = false // test new version
	tc = run(tc)
	testResult = append(testResult, tc)

}

func test() {

	// p := flag.Bool("p", false, "Run previous version")
	m := flag.Int("m", 8, "Data to send (MB)")
	flag.Parse()

	// if *p {
	// 	ncp.PreviousVersion = true
	// }

	mb := *m
	if mb <= 1 {
		mb = 1
	}
	bytesToSend := mb << 20

	time.Sleep(time.Second)

	id++
	lowTpConf := baseConf
	lowTpConf.Throughput = lowThroughput
	tc2 := &stTestCase{id: id, name: fmt.Sprintf("Append one client which has low throughput %v packets/s to base case", lowTpConf.Throughput),
		numClients: 2, bytesToSend: bytesToSend}
	tc2.mockConfigs = make(map[string]*mockconn.ConnConfig)
	tc2.mockConfigs["0"] = &baseConf
	tc2.mockConfigs["1"] = &lowTpConf

	ncp.PreviousVersion = true
	tc2 = run(tc2)
	ncp.PreviousVersion = false
	tc2 = run(tc2)
	testResult = append(testResult, tc2)

	id++
	highLatConf := baseConf
	highLatConf.Latency = highLatency
	tc3 := &stTestCase{id: id, name: fmt.Sprintf("Append one client which has high latency %v to base case", highLatConf.Latency),
		numClients: 2, bytesToSend: bytesToSend}
	tc3.mockConfigs = make(map[string]*mockconn.ConnConfig)
	tc3.mockConfigs["0"] = &baseConf
	tc3.mockConfigs["1"] = &highLatConf

	ncp.PreviousVersion = true
	tc3 = run(tc3)
	ncp.PreviousVersion = false
	tc3 = run(tc3)
	testResult = append(testResult, tc3)

	time.Sleep(2 * time.Second)

	id++
	lossConf := baseConf
	lossConf.Loss = lowLoss
	tc4 := &stTestCase{id: id, name: fmt.Sprintf("Append one client which has lowLoss %v to base case", lossConf.Loss),
		numClients: 2, bytesToSend: bytesToSend}
	tc4.mockConfigs = make(map[string]*mockconn.ConnConfig)
	tc4.mockConfigs["0"] = &baseConf
	tc4.mockConfigs["1"] = &lossConf

	ncp.PreviousVersion = true
	tc4 = run(tc4)
	ncp.PreviousVersion = false
	tc4 = run(tc4)
	testResult = append(testResult, tc4)

	time.Sleep(time.Second)

	id++
	tc5 := &stTestCase{id: id, name: "Append one low throughput and one high latency clients to base case", numClients: 3, bytesToSend: bytesToSend}
	tc5.mockConfigs = make(map[string]*mockconn.ConnConfig)
	tc5.mockConfigs["0"] = &baseConf
	tc5.mockConfigs["1"] = &lowTpConf
	tc5.mockConfigs["2"] = &highLatConf

	ncp.PreviousVersion = true
	tc5 = run(tc5)
	ncp.PreviousVersion = false
	tc5 = run(tc5)
	testResult = append(testResult, tc5)

	time.Sleep(2 * time.Second)

	id++
	tc6 := &stTestCase{id: id, name: "Append one low throughput and one lowLoss clients to base case", numClients: 3, bytesToSend: bytesToSend}
	tc6.mockConfigs = make(map[string]*mockconn.ConnConfig)
	tc6.mockConfigs["0"] = &baseConf
	tc6.mockConfigs["1"] = &lowTpConf
	tc6.mockConfigs["2"] = &lossConf

	ncp.PreviousVersion = true
	tc6 = run(tc6)
	ncp.PreviousVersion = false
	tc6 = run(tc6)
	testResult = append(testResult, tc6)

	time.Sleep(2 * time.Second)

	id++
	tc7 := &stTestCase{id: id, name: "Append one high latency and one lowLoss clients to base case", numClients: 3, bytesToSend: bytesToSend}
	tc7.mockConfigs = make(map[string]*mockconn.ConnConfig)
	tc7.mockConfigs["0"] = &baseConf
	tc7.mockConfigs["1"] = &highLatConf
	tc7.mockConfigs["2"] = &lossConf

	ncp.PreviousVersion = true
	tc7 = run(tc7)
	ncp.PreviousVersion = false
	tc7 = run(tc7)
	testResult = append(testResult, tc7)

	id++
	tc8 := &stTestCase{id: id, name: "Append one low throughput, one high latency and one lowLoss clients to base case", numClients: 4, bytesToSend: bytesToSend}
	tc8.mockConfigs = make(map[string]*mockconn.ConnConfig)
	tc8.mockConfigs["0"] = &baseConf
	tc8.mockConfigs["1"] = &lowTpConf
	tc8.mockConfigs["2"] = &highLatConf
	tc8.mockConfigs["3"] = &lossConf

	ncp.PreviousVersion = true
	tc8 = run(tc8)
	ncp.PreviousVersion = false
	tc8 = run(tc8)
	testResult = append(testResult, tc8)

	PrintResult(testResult)
	return

}

// Print test result to compare previous version and new version
func PrintResult(testResult []*stTestCase) {
	fmt.Printf("\nid \t clients \t prev speed \t new speed \t prev duration \t new duration \t test case description\n")
	for _, tc := range testResult {
		fmt.Printf("%v \t %v \t\t %.3f MB/s \t %.3f MB/s \t %v ms \t %v ms \t %v \n",
			tc.id, tc.numClients, tc.oldVersionSpeed, tc.newVersionSpeed, tc.oldVersionDuration, tc.newVersionDuration, tc.name)
	}
}

func run(tc *stTestCase) *stTestCase {

	fmt.Printf("\n>>>>>>>>>>  Test case, %v  <<<<<<<<<<\n", tc.name)

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

	if ncp.PreviousVersion {
		tc.oldVersionDuration = duration
		tc.oldVersionSpeed = speed
	} else {
		tc.newVersionDuration = duration
		tc.newVersionSpeed = speed
	}

	<-writeChan // bytesSent
	timeStart = <-writeChan
	timeEnd = <-writeChan

	testSess.localSess.PrintStatic()

	return tc
}
