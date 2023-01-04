package main

import (
	"flag"
	"fmt"
	"math/rand"
	"time"

	ncp "github.com/nknorg/ncp-go"
)

type stMockConfig struct {
	throughput int     // how many packets per second, stimulate network throughput
	latency    int     // ms, fixed latency for one connection
	minLatency int     // ms, random latency, min for one connec
	maxLatency int     // ms, random latency, min for one connec
	loss       float32 // trasmitting loss, 0.01 = 1%
}

type stTestCase struct {
	name        string
	numClients  int
	bytesToSend int
	mockConfigs map[string]*stMockConfig // map localClientID to mock config
}

const (
	lowThroughput  = 16   // packets / second
	highThroughput = 1024 // packets / second
	lowLatency     = 50   // ms
	highLatency    = 500  //ms
	loss           = 0.01 // 1% loss

	lowMinLatency  = 50  // ms
	lowMaxLatency  = 100 // ms
	highMinLatency = 300 // ms
	highMaxLatency = 800 // ms
)

func main() {
	p := flag.Bool("p", false, "Run previous version")
	m := flag.Int("m", 8, "Data to send (MB)")
	flag.Parse()

	if *p {
		ncp.PreviousVersion = true
	}

	mb := *m
	if mb <= 1 {
		mb = 1
	}
	bytesToSend := mb << 20

	baseConfig := stMockConfig{throughput: highThroughput, loss: 0, latency: lowLatency}

	tc1 := &stTestCase{name: fmt.Sprintf("1. base case, 1 client which has throughput %v packets/s , latency %v ms, loss %v",
		baseConfig.throughput, baseConfig.latency, baseConfig.loss), numClients: 1, bytesToSend: bytesToSend}
	tc1.mockConfigs = make(map[string]*stMockConfig)
	tc1.mockConfigs["0"] = &baseConfig

	run(tc1)

	time.Sleep(time.Second)

	lowTpConf := baseConfig
	lowTpConf.throughput = lowThroughput
	tc2 := &stTestCase{name: fmt.Sprintf("2. append 1 client which has low throughput %v packets/s to base case", lowTpConf.throughput),
		numClients: 2, bytesToSend: bytesToSend}
	tc2.mockConfigs = make(map[string]*stMockConfig)
	tc2.mockConfigs["0"] = &baseConfig
	tc2.mockConfigs["1"] = &lowTpConf
	run(tc2)

	highLatConf := baseConfig
	highLatConf.latency = highLatency
	tc3 := &stTestCase{name: fmt.Sprintf("3. append 1 which has high latency %v ms to base case", highLatConf.latency),
		numClients: 2, bytesToSend: bytesToSend}
	tc3.mockConfigs = make(map[string]*stMockConfig)
	tc3.mockConfigs["0"] = &baseConfig
	tc3.mockConfigs["1"] = &highLatConf
	run(tc3)

	time.Sleep(2 * time.Second)

	lossConf := baseConfig
	lossConf.loss = loss
	tc4 := &stTestCase{name: fmt.Sprintf("4. append 1 client which has loss %v to base case", lossConf.loss),
		numClients: 2, bytesToSend: bytesToSend}
	tc4.mockConfigs = make(map[string]*stMockConfig)
	tc4.mockConfigs["0"] = &baseConfig
	tc4.mockConfigs["1"] = &lossConf
	run(tc4)

	return

	time.Sleep(time.Second)

	tc5 := &stTestCase{name: "5. append low throughput and high latency clients to base case", numClients: 3, bytesToSend: bytesToSend}
	tc5.mockConfigs = make(map[string]*stMockConfig)
	tc5.mockConfigs["0"] = &baseConfig
	tc5.mockConfigs["1"] = &lowTpConf
	tc5.mockConfigs["2"] = &highLatConf
	run(tc5)

	time.Sleep(2 * time.Second)

	tc6 := &stTestCase{name: "6. append low throughput and loss clients to base case", numClients: 3, bytesToSend: bytesToSend}
	tc6.mockConfigs = make(map[string]*stMockConfig)
	tc6.mockConfigs["0"] = &baseConfig
	tc6.mockConfigs["1"] = &lowTpConf
	tc6.mockConfigs["2"] = &lossConf
	run(tc6)

	time.Sleep(2 * time.Second)

	tc7 := &stTestCase{name: "7. append high latency and loss clients to base case", numClients: 3, bytesToSend: bytesToSend}
	tc7.mockConfigs = make(map[string]*stMockConfig)
	tc7.mockConfigs["0"] = &baseConfig
	tc7.mockConfigs["1"] = &highLatConf
	tc7.mockConfigs["2"] = &lossConf
	run(tc7)

}

func run(tc *stTestCase) {
	fmt.Printf("\n>>>>>>>>>>  Test case: %v  <<<<<<<<<<\n", tc.name)

	testSess := TestSession{}
	testSess.Create(tc.mockConfigs, tc.numClients)

	testSess.DialUp()

	writeChan := make(chan int64, 3)
	readChan := make(chan int64, 3)

	go testSess.write(testSess.sendSess, tc.bytesToSend, writeChan)
	go testSess.read(testSess.recvSess, readChan)

	bytesReceived := <-readChan
	timeStart := <-readChan
	timeEnd := <-readChan

	fmt.Printf("\n%v received %v bytes at %.3f MB/s, duration: %v ms. \n",
		testSess.recvSess.LocalAddr(), bytesReceived,
		float64(bytesReceived)/(1<<20)/(float64((timeEnd-timeStart))/1000.0),
		timeEnd-timeStart)
	testSess.recvSess.PrintStatic()

	<-writeChan // bytesSent
	timeStart = <-writeChan
	timeEnd = <-writeChan

	// fmt.Printf("\n%v sent %v bytes at %.3f MB/s, duration %v ms \n",
	// 	testSess.sendSess.LocalAddr(), bytesSent,
	// 	(float64(bytesSent)/(1<<20))/(float64(timeEnd-timeStart)/1000.0),
	// 	timeEnd-timeStart)
	testSess.sendSess.PrintStatic()

}

func randDropAndLatency(conf *stMockConfig, localId, remoteId string) bool {

	rand.Seed(time.Now().UnixNano())
	loss := rand.Float32()
	latency := conf.minLatency + int(rand.Float32()*float32(conf.maxLatency-conf.minLatency))

	if loss > conf.loss {
		time.Sleep(time.Duration(latency) * time.Millisecond)
		return false // no drop, just get latency

	}

	return true // droped, loss
}

// this main is use fifo slice to contronl throughput, abondoned.
func main0() {
	p := flag.Bool("p", false, "Run previous version")
	m := flag.Int("m", 8, "Data to send (MB)")
	flag.Parse()

	if *p {
		ncp.PreviousVersion = true
	}

	mb := *m
	if mb <= 1 {
		mb = 1
	}
	bytesToSend := mb << 20

	baseConfig := stMockConfig{throughput: highThroughput, loss: 0, minLatency: lowMinLatency, maxLatency: lowMaxLatency}

	tc1 := &stTestCase{name: "base case, 1 client which has high throughput, low latency, zero loss", numClients: 1, bytesToSend: bytesToSend}
	tc1.mockConfigs = make(map[string]*stMockConfig)
	tc1.mockConfigs["0"] = &baseConfig

	run(tc1)

	time.Sleep(time.Second)

	tc2 := &stTestCase{name: "2 clients, append 1 low throughput client to base case", numClients: 2, bytesToSend: bytesToSend}
	tc2.mockConfigs = make(map[string]*stMockConfig)
	tc2.mockConfigs["0"] = &baseConfig
	lowTpConf := baseConfig
	lowTpConf.throughput = lowThroughput
	tc2.mockConfigs["1"] = &lowTpConf
	run(tc2)

	tc3 := &stTestCase{name: "2 clients, append 1 high latency client to base case", numClients: 2, bytesToSend: bytesToSend}
	tc3.mockConfigs = make(map[string]*stMockConfig)
	tc3.mockConfigs["0"] = &baseConfig
	highLatConf := baseConfig
	highLatConf.minLatency = highMinLatency
	highLatConf.maxLatency = highMaxLatency
	tc3.mockConfigs["1"] = &highLatConf
	run(tc3)

	time.Sleep(2 * time.Second)

	tc4 := &stTestCase{name: "2 clients, append 1 loss client to base case", numClients: 2, bytesToSend: bytesToSend}
	tc4.mockConfigs = make(map[string]*stMockConfig)
	tc4.mockConfigs["0"] = &baseConfig
	lossConf := baseConfig
	lossConf.loss = loss
	tc4.mockConfigs["1"] = &lossConf
	run(tc4)

	time.Sleep(time.Second)

	tc5 := &stTestCase{name: "3 clients, append low throughput and high latency clients to base case", numClients: 3, bytesToSend: bytesToSend}
	tc5.mockConfigs = make(map[string]*stMockConfig)
	tc5.mockConfigs["0"] = &baseConfig
	tc5.mockConfigs["1"] = &lowTpConf
	tc5.mockConfigs["2"] = &highLatConf
	run(tc5)

	time.Sleep(2 * time.Second)

	tc6 := &stTestCase{name: "3 clients, append low throughput and loss clients to base case", numClients: 3, bytesToSend: bytesToSend}
	tc6.mockConfigs = make(map[string]*stMockConfig)
	tc6.mockConfigs["0"] = &baseConfig
	tc6.mockConfigs["1"] = &lowTpConf
	tc6.mockConfigs["2"] = &lossConf
	run(tc6)

	time.Sleep(2 * time.Second)

	tc7 := &stTestCase{name: "3 clients, append high latency and loss clients to base case", numClients: 3, bytesToSend: bytesToSend}
	tc7.mockConfigs = make(map[string]*stMockConfig)
	tc7.mockConfigs["0"] = &baseConfig
	tc7.mockConfigs["1"] = &highLatConf
	tc7.mockConfigs["2"] = &lossConf
	run(tc7)

}
