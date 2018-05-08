package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"testing"
	"time"
)


type testHTTPMock struct {
	ln	 				*net.TCPListener
	messages        	[]map[string]interface{}
	batch				int
	test 				*testing.T
	statusCodes			[]int
	token				string
	constStatusCodeFlag	bool
	constStatusCode		int
	lastMessageTime		chan time.Time
	lastLog				string
}


func NewtestHTTPMock(t *testing.T, returnStatusCodes []int) *testHTTPMock {
	laddr := &net.TCPAddr{IP: []byte{127, 0, 0, 1}, Port: 0, Zone: ""}
	ln, err := net.ListenTCP("tcp", laddr)
	if err != nil {
		t.Fatal(err)
	}
	return &testHTTPMock{
		ln:         		ln,
		batch:				0,
		test:           	t,
		statusCodes:		returnStatusCodes,
		token:				"123456789",
		lastMessageTime: 	make(chan time.Time, 5000),
	}
}


func (m *testHTTPMock) Serve() error {
	return http.Serve(m.ln, m)
}


//type Handler interface {
//	ServeHTTP(ResponseWriter, *Request)
//}
func (m *testHTTPMock) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodPost:
		if fmt.Sprintf("%s%s", "http://",request.Host) != m.URL() {
			m.test.Errorf("Wrong URL - %v", request.Host)
		}
		lastMessageTime := time.Now()
		defer request.Body.Close()
		reqBody := request.Body
		body, err := ioutil.ReadAll(reqBody)
		if err != nil {
			m.test.Fatal(err)
		}

		jsonStart := 0
		for jsonEnd := 0; jsonEnd < len(body); jsonEnd++ {
			if jsonEnd == len(body)-2 || (body[jsonEnd] == '}' && body[jsonEnd+2] == '{') {
				var message map[string]interface{}
				err = json.Unmarshal(body[jsonStart:jsonEnd+1], &message)
				if err != nil {
					m.test.Log(string(body[jsonStart : jsonEnd+1]))
					m.test.Fatal(err)
				}
				m.test.Logf("mock received message: %s", string(string(body[jsonStart : jsonEnd+1])))
				m.messages = append(m.messages, message)
				jsonStart = jsonEnd + 1
				if m.lastLog != ""{
					if message["message"] == m.lastLog{
						m.lastMessageTime <- lastMessageTime
					}
				}
			}
		}

		if m.constStatusCodeFlag{
			writer.WriteHeader(m.constStatusCode)
		}else{
			if m.batch > len(m.statusCodes) {
				m.test.Fatalf("Not enough response status codes configured")
			}
			retStatus := m.statusCodes[m.batch]
			writer.WriteHeader(retStatus)
		}
		m.batch++
	default:
		m.test.Errorf("Unexpected HTTP method %s", request.Method)
		writer.WriteHeader(http.StatusBadRequest)
	}
}

func (m *testHTTPMock) setLastLog(lastLog string){
	m.lastLog = lastLog
}

func (m *testHTTPMock) Token() string{
	return m.token
}


func (m *testHTTPMock) Batch() int{
	return m.batch
}

func (m *testHTTPMock) URL() string {
	return "http://" + m.ln.Addr().String()
}


func (m *testHTTPMock) Close() error {
	close(m.lastMessageTime)
	return m.ln.Close()
}

func (m *testHTTPMock) setStatusCode(status int){
	m.constStatusCode = status
	m.constStatusCodeFlag = true
}