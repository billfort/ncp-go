package test

import (
	"bytes"
	"fmt"
)

type TestData struct {
	TestSend int64
	SessSend int64
	ConnSend int64
	ConnRecv int64
	SessRecv int64
	TestRecv int64
}

func (d TestData) Enc() []byte {
	var b bytes.Buffer
	fmt.Fprintln(&b, d.TestSend, d.SessSend, d.ConnSend, d.ConnRecv, d.SessRecv, d.TestRecv)
	return b.Bytes()
}
func ReplaceTestData(b []byte, d []byte) {
	if len(d) == 0 || len(b) == 0 || len(b) < len(d) {
		return
	}
	for i := 0; i < len(d); i++ {
		b[i] = d[i]
	}
}
func (d *TestData) Dec(b []byte) error {
	buf := bytes.NewBuffer(b)
	_, err := fmt.Fscanln(buf, &d.TestSend, &d.SessSend, &d.ConnSend, &d.ConnRecv, &d.SessRecv, &d.TestRecv)
	return err
}
