package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/nknorg/ncp-go/test/mockconn"

	ncp "github.com/nknorg/ncp-go"
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
	lowThroughput  = 16                     // packets / second
	highThroughput = 1024                   // packets / second
	lowLatency     = 50 * time.Millisecond  // ms
	highLatency    = 500 * time.Millisecond //ms
	loss           = 0.01                   // 1% loss
)

func main() {

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

	testResult := make([]*stTestCase, 0)

	baseConfig := mockconn.ConnConfig{Throughput: highThroughput, Loss: 0, Latency: lowLatency}

	id := 1
	tc1 := &stTestCase{id: id, name: fmt.Sprintf("Base case, one client throughput %v packets/s , latency %v, loss %v",
		baseConfig.Throughput, baseConfig.Latency, baseConfig.Loss), numClients: 1, bytesToSend: bytesToSend}
	tc1.mockConfigs = make(map[string]*mockconn.ConnConfig)
	tc1.mockConfigs["0"] = &baseConfig

	ncp.PreviousVersion = true
	tc1 = run(tc1)
	ncp.PreviousVersion = false
	tc1 = run(tc1)
	testResult = append(testResult, tc1)

	time.Sleep(time.Second)

	id++
	lowTpConf := baseConfig
	lowTpConf.Throughput = lowThroughput
	tc2 := &stTestCase{id: id, name: fmt.Sprintf("Append 1 client which has low throughput %v packets/s to base case", lowTpConf.Throughput),
		numClients: 2, bytesToSend: bytesToSend}
	tc2.mockConfigs = make(map[string]*mockconn.ConnConfig)
	tc2.mockConfigs["0"] = &baseConfig
	tc2.mockConfigs["1"] = &lowTpConf

	ncp.PreviousVersion = true
	tc2 = run(tc2)
	ncp.PreviousVersion = false
	tc2 = run(tc2)
	testResult = append(testResult, tc2)

	id++
	highLatConf := baseConfig
	highLatConf.Latency = highLatency
	tc3 := &stTestCase{id: id, name: fmt.Sprintf("Append 1 client which has high latency %v to base case", highLatConf.Latency),
		numClients: 2, bytesToSend: bytesToSend}
	tc3.mockConfigs = make(map[string]*mockconn.ConnConfig)
	tc3.mockConfigs["0"] = &baseConfig
	tc3.mockConfigs["1"] = &highLatConf

	ncp.PreviousVersion = true
	tc3 = run(tc3)
	ncp.PreviousVersion = false
	tc3 = run(tc3)
	testResult = append(testResult, tc3)

	time.Sleep(2 * time.Second)

	id++
	lossConf := baseConfig
	lossConf.Loss = loss
	tc4 := &stTestCase{id: id, name: fmt.Sprintf("Append 1 client which has loss %v to base case", lossConf.Loss),
		numClients: 2, bytesToSend: bytesToSend}
	tc4.mockConfigs = make(map[string]*mockconn.ConnConfig)
	tc4.mockConfigs["0"] = &baseConfig
	tc4.mockConfigs["1"] = &lossConf

	ncp.PreviousVersion = true
	tc4 = run(tc4)
	ncp.PreviousVersion = false
	tc4 = run(tc4)
	testResult = append(testResult, tc4)

	time.Sleep(time.Second)

	id++
	tc5 := &stTestCase{id: id, name: "Append low throughput and high latency clients to base case", numClients: 3, bytesToSend: bytesToSend}
	tc5.mockConfigs = make(map[string]*mockconn.ConnConfig)
	tc5.mockConfigs["0"] = &baseConfig
	tc5.mockConfigs["1"] = &lowTpConf
	tc5.mockConfigs["2"] = &highLatConf

	ncp.PreviousVersion = true
	tc5 = run(tc5)
	ncp.PreviousVersion = false
	tc5 = run(tc5)
	testResult = append(testResult, tc5)

	time.Sleep(2 * time.Second)

	id++
	tc6 := &stTestCase{id: id, name: "Append low throughput and loss clients to base case", numClients: 3, bytesToSend: bytesToSend}
	tc6.mockConfigs = make(map[string]*mockconn.ConnConfig)
	tc6.mockConfigs["0"] = &baseConfig
	tc6.mockConfigs["1"] = &lowTpConf
	tc6.mockConfigs["2"] = &lossConf

	ncp.PreviousVersion = true
	tc6 = run(tc6)
	ncp.PreviousVersion = false
	tc6 = run(tc6)
	testResult = append(testResult, tc6)

	time.Sleep(2 * time.Second)

	id++
	tc7 := &stTestCase{id: id, name: "Append high latency and loss clients to base case", numClients: 3, bytesToSend: bytesToSend}
	tc7.mockConfigs = make(map[string]*mockconn.ConnConfig)
	tc7.mockConfigs["0"] = &baseConfig
	tc7.mockConfigs["1"] = &highLatConf
	tc7.mockConfigs["2"] = &lossConf

	ncp.PreviousVersion = true
	tc7 = run(tc7)
	ncp.PreviousVersion = false
	tc7 = run(tc7)
	testResult = append(testResult, tc7)

	fmt.Printf("id \t clients \t prev speed \t new speed \t prev duration \t new duration \t note\n")
	for _, tc := range testResult {
		fmt.Printf("%v \t %v \t\t %.3f MB/s \t %.3f MB/s \t %v ms \t %v ms \t %v \n",
			tc.id, tc.numClients, tc.oldVersionSpeed, tc.newVersionSpeed, tc.oldVersionDuration, tc.newVersionDuration, tc.name)
	}

}

func run(tc *stTestCase) *stTestCase {

	fmt.Printf("\n>>>>>>>>>>  Test case %v  <<<<<<<<<<\n", tc.name)

	var testSess TestSession
	testSess.Create(tc.mockConfigs, tc.numClients)

	testSess.DialUp()

	writeChan := make(chan int64, 3)
	readChan := make(chan int64, 3)

	go testSess.write(testSess.localSess, tc.bytesToSend, writeChan)
	go testSess.read(testSess.remoteSess, readChan)

	bytesReceived := <-readChan
	timeStart := <-readChan
	timeEnd := <-readChan

	speed := float64(bytesReceived) / (1 << 20) / (float64((timeEnd - timeStart)) / 1000.0)
	duration := timeEnd - timeStart
	fmt.Printf("\n%v received %v bytes at %.3f MB/s, duration: %v. \n",
		testSess.remoteSess.LocalAddr(), bytesReceived, speed, duration)
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
