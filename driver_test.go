package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/beeker1121/goque"
	"github.com/docker/docker/daemon/logger"

	"net/http"
	"net/http/httptest"
)

func TestValidateDriverOpt(t *testing.T) {
	conf := map[string]string{
		logzioFormat:    "json",
		logzioLogSource: "logzioSource",
		logzioTag:       "logzioTag",
		logzioToken:     "logzioToken",
		logzioType:      "logzioType",
		logzioURL:       "logzioURL",
		logzioDirPath:   fmt.Sprintf("./%s", t.Name()),
		logzioLogAttr:   `{"num":6.13,"str":"str"}`,
		envRegex:        "reg",
		dockerLabels:    "label",
		dockerEnv:       "env",
	}
	info := logger.Info{
		ContainerID: "123456789",
		Config:      conf,
	}

	if _, err := validateDriverOpt(info); err != nil {
		t.Fatalf("TestValidDriverOpt: %s", err)
	}
}

func TestMissingToken(t *testing.T) {
	conf := map[string]string{
		logzioURL:     "logzioURL",
		logzioDirPath: fmt.Sprintf("./%s", t.Name()),
	}

	info := logger.Info{
		ContainerID: "123456789",
		Config:      conf,
	}

	if _, err := validateDriverOpt(info); err.Error() != "logz.io token is required\n" {
		t.Fatalf("Failed TestMissingToken, got wrong error: %s", err)
	}
}

func TestNoSuchLogOpt(t *testing.T) {
	conf := map[string]string{
		logzioFormat:    "json",
		logzioLogSource: "logzioSource",
		logzioTag:       "logzioTag",
		logzioToken:     "logzioToken",
		logzioType:      "logzioType",
		logzioURL:       "logzioURL",
		logzioDirPath:   fmt.Sprintf("./%s", t.Name()),
		"logzioDummy":   "logzioDummy",
	}

	info := logger.Info{
		ContainerID: "123456789",
		Config:      conf,
	}

	if _, err := validateDriverOpt(info); err.Error() != "wrong log-opt: 'logzioDummy' - 123456789\n" {
		t.Fatalf("Failed TestNoSuchLogOpt got wrong error: %s", err)
	}
}

func TestSendingString(t *testing.T) {
	mock := NewtestHTTPMock(t, []int{http.StatusOK, http.StatusOK})
	go mock.Serve()
	defer mock.Close()
	info := logger.Info{
		Config: map[string]string{
			logzioURL:     mock.URL(),
			logzioToken:   mock.Token(),
			logzioFormat:  defaultFormat,
			logzioDirPath: fmt.Sprintf("./%s", t.Name()),
			logzioLogAttr: `{"num":6.13,"str":"str"}`,
			logzioTag:     "{{.ImageName}}",
			dockerLabels:  "labelKey",
			envRegex:      "^envKey",
		},
		ContainerID:        "containeriid",
		ContainerName:      "/container_name",
		ContainerImageID:   "contaimageid",
		ContainerImageName: "container_image_name",
		ContainerEnv:       []string{"envKey=envValue"},
		ContainerLabels: map[string]string{
			"labelKey": "labelValue",
		},
	}

	logziol, err := newLogzioLogger(info, nil, "0")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(info.Config[logzioDirPath])

	str := &logger.Message{
		Line:         []byte("string"),
		Source:       "stdout",
		Timestamp:    time.Now(),
		Partial: 	  false,
	}
	if err := logziol.Log(str); err != nil {
		t.Fatalf("Failed Log string: %s", err)
	}

	err = logziol.Close()
	if err != nil {
		t.Fatal(err)
	}

	hostname, err := info.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	// check string message
	sm := mock.messages[0]
	if sm["hostname"] != hostname ||
		sm["log_source"] != "stdout" ||
		sm["tags"] != info.ContainerID ||
		sm["type"] != defaultSourceType ||
		sm["driver_timestamp"] != time.Unix(0, str.Timestamp.UnixNano()).Format(time.RFC3339Nano) {
		t.Fatalf("Failed string message, one of the meata data is missing. %+v\n", sm)
	}
	if sm["envKey"] != "envValue" || sm["labelKey"] != "labelValue" {
		t.Fatalf("Failed string message, one of the Extra fields is wrong. %+v\n", sm)
	}

	if sm["num"] != 6.13 || sm["str"] != "str" {
		t.Fatalf("Failed string message, one of the attributes fields is wrong. %+v\n", sm)
	}
	if sm["message"] != "string" {
		t.Fatalf("Failed string message, not a string: %s", sm["message"])
	}
}

func TestSendingJson(t *testing.T) {
	mock := NewtestHTTPMock(t, []int{http.StatusOK, http.StatusOK})
	go mock.Serve()
	defer mock.Close()
	info := logger.Info{
		Config: map[string]string{
			logzioURL:     mock.URL(),
			logzioToken:   mock.Token(),
			logzioFormat:  jsonFormat,
			logzioDirPath: fmt.Sprintf("./%s", t.Name()),
			logzioTag:     "{{.ImageName}}",
			dockerLabels:  "labelKey",
			envRegex:      "^envKey",
		},
		ContainerID:        "containeriid",
		ContainerName:      "/container_name",
		ContainerImageID:   "contaimageid",
		ContainerImageName: "container_image_name",
		ContainerEnv:       []string{"envKey=envValue"},
		ContainerLabels: map[string]string{
			"labelKey": "labelValue",
		},
	}

	logziol, err := newLogzioLogger(info, nil, "0")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(info.Config[logzioDirPath])

	jstr := &logger.Message{
		Line:         []byte("{\"key\":\"value\"}"),
		Source:       "stdout",
		Timestamp:    time.Now(),
		Partial: 	  false,
	}

	if err := logziol.Log(jstr); err != nil {
		t.Fatalf("Failed Log json: %s", err)
	}

	err = logziol.Close()
	if err != nil {
		t.Fatal(err)
	}

	hostname, err := info.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	// check json message
	jm := mock.messages[0]
	if jm["hostname"] != hostname ||
		jm["log_source"] != "stdout" ||
		jm["tags"] != info.ContainerID ||
		jm["type"] != defaultSourceType ||
		jm["driver_timestamp"] != time.Unix(0, jstr.Timestamp.UnixNano()).Format(time.RFC3339Nano) {
		t.Fatalf("Failed string message, one of the meata data is missing. %+v\n", jm)
	}
	if jm["envKey"] != "envValue" || jm["labelKey"] != "labelValue" {
		t.Fatalf("Failed string message, one of the Extra fields is wrong. %+v\n", jm)
	}

	// casting message to a map
	if jm["message"].(map[string]interface{})["key"] != "value" {
		t.Fatalf("Failed json message, not a json: %v", jm["message"])
	}
}

func TestStress(t *testing.T) {
	mock := NewtestHTTPMock(t, []int{http.StatusOK})
	mock.setStatusCode(http.StatusOK)
	go mock.Serve()
	defer mock.Close()
	os.Setenv(envLogsDrainTimeout, "1s")
	defer os.Setenv(envLogsDrainTimeout, "5s")
	info := logger.Info{
		Config: map[string]string{
			logzioURL:     mock.URL(),
			logzioToken:   mock.Token(),
			logzioFormat:  defaultFormat,
			logzioDirPath: fmt.Sprintf("./%s", t.Name()),
			logzioLogAttr: `{"num":6.13,"str":"str"}`,
			logzioTag:     "{{.ImageName}}",
			dockerLabels:  "labelKey",
			envRegex:      "^envKey",
		},
		ContainerID:        "containeriid",
		ContainerName:      "/container_name",
		ContainerImageID:   "contaimageid",
		ContainerImageName: "container_image_name",
		ContainerEnv:       []string{"envKey=envValue"},
		ContainerLabels: map[string]string{
			"labelKey": "labelValue",
		},
	}
	totalLogs := 10000
	logziol, err := newLogzioLogger(info, nil, "0")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(info.Config[logzioDirPath])

	for i := 0; i < totalLogs; i++ {
		if err := logziol.Log(&logger.Message{Line: []byte(fmt.Sprintf("%s%d", "str", i)), Source: "stdout",
			Timestamp: time.Now(), Partial: false}); err != nil {
			t.Fatalf("Failed Log string: %s", err)
		}
	}

	time.Sleep(10 * time.Second)
	err = logziol.Close()
	if err != nil {
		t.Fatal(err)
	}

	hostname, err := info.Hostname()
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < totalLogs; i++ {
		// check string message
		sm := mock.messages[i]
		if sm["hostname"] != hostname ||
			sm["log_source"] != "stdout" ||
			sm["tags"] != info.ContainerID ||
			sm["type"] != defaultSourceType {
			t.Fatalf("Failed string message, one of the meata data is missing. %+v\n", sm)
		}
		if sm["envKey"] != "envValue" || sm["labelKey"] != "labelValue" {
			t.Fatalf("Failed string message, one of the Extra fields is wrong. %+v\n", sm)
		}

		if sm["num"] != 6.13 || sm["str"] != "str" {
			t.Fatalf("Failed string message, one of the attributes fields is wrong. %+v\n", sm)
		}
		testStr := fmt.Sprintf("%s%d", "str", i)
		if sm["message"] != testStr {
			t.Fatalf("Failed to find: %s instead found %s total size is %d", testStr, sm["message"], len(mock.messages))
		}
	}

}

func TestSendingEmptyString(t *testing.T) {
	mock := NewtestHTTPMock(t, []int{http.StatusOK, http.StatusOK})
	go mock.Serve()
	defer mock.Close()
	info := logger.Info{
		Config: map[string]string{
			logzioURL:     mock.URL(),
			logzioToken:   mock.Token(),
			logzioFormat:  defaultFormat,
			logzioDirPath: fmt.Sprintf("./%s", t.Name()),
			logzioLogAttr: `{"num":6.13,"str":"str"}`,
			logzioTag:     "{{.ImageName}}",
			dockerLabels:  "labelKey",
			envRegex:      "^envKey",
		},
		ContainerID:        "containeriid",
		ContainerName:      "/container_name",
		ContainerImageID:   "contaimageid",
		ContainerImageName: "container_image_name",
		ContainerEnv:       []string{"envKey=envValue"},
		ContainerLabels: map[string]string{
			"labelKey": "labelValue",
		},
	}

	logziol, err := newLogzioLogger(info, nil, "0")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(info.Config[logzioDirPath])

	stringOne := "Logz.io !"
	stringTwo := "   "
	stringThree := ""

	str := &logger.Message{
		Line:         []byte(stringOne),
		Source:       "stdout",
		Timestamp:    time.Now(),
		Partial: 	  false,
	}
	if err := logziol.Log(str); err != nil {
		t.Fatalf("Failed Log string: %s", err)
	}

	str.Line = []byte(stringTwo)
	if err := logziol.Log(str); err != nil {
		t.Fatalf("Failed Log string: %s", err)
	}

	str.Line = []byte(stringThree)
	if err := logziol.Log(str); err != nil {
		t.Fatalf("Failed Log string: %s", err)
	}

	err = logziol.Close()
	if err != nil {
		t.Fatal(err)
	}

	hostname, err := info.Hostname()
	if err != nil {
		t.Fatal(err)
	}

	if len(mock.messages) != 1 {
		t.Fatalf("Failed to ignore empty messages. Expected 1 but found %d\n", len(mock.messages))
	}

	// check string message
	sm := mock.messages[0]
	if sm["hostname"] != hostname ||
		sm["log_source"] != "stdout" ||
		sm["tags"] != info.ContainerID ||
		sm["type"] != defaultSourceType ||
		sm["driver_timestamp"] != time.Unix(0, str.Timestamp.UnixNano()).Format(time.RFC3339Nano) {
		t.Fatalf("Failed string message, one of the meata data is missing. %+v\n", sm)
	}
	if sm["envKey"] != "envValue" || sm["labelKey"] != "labelValue" {
		t.Fatalf("Failed string message, one of the Extra fields is wrong. %+v\n", sm)
	}

	if sm["num"] != 6.13 || sm["str"] != "str" {
		t.Fatalf("Failed string message, one of the attributes fields is wrong. %+v\n", sm)
	}
	if sm["message"] != stringOne {
		t.Fatalf("Failed string message, not a string: %s", sm["message"])
	}
}

func TestSendingNoTag(t *testing.T) {
	mock := NewtestHTTPMock(t, []int{http.StatusOK, http.StatusOK})
	go mock.Serve()
	defer mock.Close()
	info := logger.Info{
		Config: map[string]string{
			logzioURL:     mock.URL(),
			logzioToken:   mock.Token(),
			logzioFormat:  jsonFormat,
			logzioDirPath: fmt.Sprintf("./%s", t.Name()),
			logzioTag:     "",
		},
		ContainerID:        "containeriid",
		ContainerName:      "/container_name",
		ContainerImageID:   "contaimageid",
		ContainerImageName: "container_image_name",
	}
	logziol, err := newLogzioLogger(info, nil, "0")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(info.Config[logzioDirPath])

	str := &logger.Message{
		Line:         []byte("string"),
		Source:       "stdout",
		Timestamp:    time.Now(),
		Partial: 	  false,
	}

	if err := logziol.Log(str); err != nil {
		t.Fatalf("Failed Log string: %s", err)
	}
	err = logziol.Close()
	if err != nil {
		t.Fatal(err)
	}

	hostname, err := info.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	// check string message
	sm := mock.messages[0]
	if sm["hostname"] != hostname ||
		sm["log_source"] != "stdout" ||
		sm["type"] != defaultSourceType ||
		sm["driver_timestamp"] != time.Unix(0, str.Timestamp.UnixNano()).Format(time.RFC3339Nano) {
		t.Fatalf("Failed string message, one of the meata data is missing. %+v\n", sm)
	}

	if sm["message"] != "string" {
		t.Fatalf("Failed string message, not a string: %s", sm["message"])
	}
}

func TestTimerSendingNotExpired(t *testing.T) {
	mock := NewtestHTTPMock(t, []int{http.StatusOK, http.StatusOK})
	go mock.Serve()
	defer mock.Close()

	info := logger.Info{
		Config: map[string]string{
			logzioURL:     mock.URL(),
			logzioToken:   mock.Token(),
			logzioFormat:  jsonFormat,
			logzioDirPath: fmt.Sprintf("./%s", t.Name()),
		},
		ContainerID:        "containeriid",
		ContainerName:      "/container_name",
		ContainerImageID:   "contaimageid",
		ContainerImageName: "container_image_name",
	}
	logziol, err := newLogzioLogger(info, nil, "0")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(info.Config[logzioDirPath])

	msgTime := time.Now()
	str := &logger.Message{
		Line:         []byte("string"),
		Source:       "stdout",
		Timestamp:    msgTime,
		Partial: 	  false,
	}

	if err := logziol.Log(str); err != nil {
		t.Fatalf("Failed Log string: %s", err)
	}

	<-time.After(defaultLogsDrainTimeout - (time.Second * 1))

	str = &logger.Message{
		Line:         []byte("string"),
		Source:       "stdout",
		Timestamp:    msgTime,
		Partial: 	  false,
	}

	if err := logziol.Log(str); err != nil {
		t.Fatalf("Failed Log string: %s", err)
	}

	err = logziol.Close()
	if err != nil {
		t.Fatal(err)
	}

	<-time.After(250 * time.Millisecond)

	batchNumber := mock.Batch()
	if batchNumber != 1 {
		t.Fatalf("Unexpected batch number %d. Expected 1", batchNumber)
	}
	// sanity check
	firstMsg, secondMsg := mock.messages[0], mock.messages[1]
	if firstMsg["message"] != secondMsg["message"] ||
		firstMsg["log_source"] != secondMsg["log_source"] ||
		firstMsg["hostname"] != secondMsg["hostname"] ||
		firstMsg["driver_timestamp"] != secondMsg["driver_timestamp"] ||
		firstMsg["tags"] != secondMsg["tags"] {
		t.Fatal("Expecting messages to be the same")
	}
}

func TestTimerSendingExpired(t *testing.T) {
	mock := NewtestHTTPMock(t, []int{http.StatusOK, http.StatusOK})
	go mock.Serve()
	defer mock.Close()

	info := logger.Info{
		Config: map[string]string{
			logzioURL:     mock.URL(),
			logzioToken:   mock.Token(),
			logzioFormat:  jsonFormat,
			logzioDirPath: fmt.Sprintf("./%s", t.Name()),
		},
		ContainerID:        "containeriid",
		ContainerName:      "/container_name",
		ContainerImageID:   "contaimageid",
		ContainerImageName: "container_image_name",
	}
	logziol, err := newLogzioLogger(info, nil, "0")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(info.Config[logzioDirPath])

	msgTime := time.Now()
	str := &logger.Message{
		Line:         []byte("string"),
		Source:       "stdout",
		Timestamp:    msgTime,
		Partial: 	  false,
	}

	if err := logziol.Log(str); err != nil {
		t.Fatalf("Failed Log string: %s", err)
	}

	<-time.After(defaultLogsDrainTimeout + (time.Second * 1))

	str = &logger.Message{
		Line:         []byte("string"),
		Source:       "stdout",
		Timestamp:    msgTime,
		Partial: 	  false,
	}

	if err := logziol.Log(str); err != nil {
		t.Fatalf("Failed Log string: %s", err)
	}

	err = logziol.Close()
	if err != nil {
		t.Fatal(err)
	}

	<-time.After(250 * time.Millisecond)

	batchNumber := mock.Batch()
	if batchNumber != 2 {
		t.Fatalf("Unexpected batch number %d. Expected 1", batchNumber)
	}
	// sanity check
	sMsg, jMsg := mock.messages[0], mock.messages[1]
	if sMsg["message"] != jMsg["message"] ||
		sMsg["log_source"] != jMsg["log_source"] ||
		sMsg["hostname"] != jMsg["hostname"] ||
		sMsg["driver_timestamp"] != jMsg["driver_timestamp"] ||
		sMsg["tags"] != jMsg["tags"] {
		t.Fatal("Expecting messages to be the same")
	}
}

func TestDrainAfterClosed(t *testing.T) {
	resp := []int{http.StatusInternalServerError, http.StatusInternalServerError, http.StatusOK}
	mock := NewtestHTTPMock(t, resp)
	go mock.Serve()
	defer mock.Close()
	info := logger.Info{
		Config: map[string]string{
			logzioURL:     mock.URL(),
			logzioToken:   mock.Token(),
			logzioFormat:  jsonFormat,
			logzioDirPath: fmt.Sprintf("./%s", t.Name()),
		},
		ContainerID:        "containeriid",
		ContainerName:      "/container_name",
		ContainerImageID:   "contaimageid",
		ContainerImageName: "container_image_name",
	}
	if err := os.Setenv(envLogsDrainTimeout, "60s"); err != nil {
		t.Fatal(err)
	}

	logziol, err := newLogzioLogger(info, nil, "0")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(info.Config[logzioDirPath])

	for i := 0; i < 5; i++ {
		if err := logziol.Log(&logger.Message{Line: []byte(fmt.Sprintf("%s%d", "str", i)), Source: "stdout",
			Timestamp: time.Now(), Partial: false}); err != nil {
			t.Fatalf("Failed Log string: %s", err)
		}
	}

	err = logziol.Close()
	if err != nil {
		t.Fatal(err)
	}

	batchNumber := mock.Batch()
	if batchNumber != len(resp) {
		t.Fatalf("Unexpected batch number %d. Expected %d retries", batchNumber, 3)
	}

	// sanity check
	for i := 0; i < 5; i++ {
		if mock.messages[i]["message"] != fmt.Sprintf("%s%d", "str", i) {
			t.Fatalf("message %g not matching message %d", mock.messages[i]["message"], i)
		}
	}
}

func TestServerIsDown(t *testing.T) {
	var sent = make([]byte, 1024)
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		r.Body.Read(sent)
	}))
	defer ts.Close()
	info := logger.Info{
		Config: map[string]string{
			logzioURL:     "http://localhost:12345",
			logzioToken:   "123456789",
			logzioFormat:  jsonFormat,
			logzioDirPath: fmt.Sprintf("./%s", t.Name()),
		},
		ContainerID:        "containeriid",
		ContainerName:      "/container_name",
		ContainerImageID:   "contaimageid",
		ContainerImageName: "container_image_name",
	}
	logziol, err := newLogzioLogger(info, nil, "0")
	if err != nil {
		t.Fatal(err)
	}

	defer os.RemoveAll(info.Config[logzioDirPath])

	for i := 0; i < 5; i++ {
		if err := logziol.Log(&logger.Message{Line: []byte(fmt.Sprintf("%s%d", t.Name(), i)),
			Source: "stdout", Timestamp: time.Now(), Partial: false}); err != nil {
			t.Fatalf("Failed Log string: %s", err)
		}
	}

	err = logziol.Close()
	if err != nil {
		t.Fatal(err)
	}

	//check the logs are saved to disk
	q, err := goque.OpenQueue(fmt.Sprintf("./%s/0", t.Name()))
	//We requeue as one big string
	if q.Length() != 1 {
		t.Fatalf("Queue length is not as expected: %d", q.Length())
	}
	item, errQ := q.Dequeue()
	if errQ != nil {
		t.Fatal(errQ)
	}
	bytesReader := bytes.NewBuffer(item.Value)
	bufReader := bufio.NewReader(bytesReader)
	for i := 0; i < 5; i++ {
		var data map[string]interface{}
		msg, _, _ := bufReader.ReadLine()
		err := json.Unmarshal([]byte(msg), &data)
		if err != nil {
			panic(err)
		}
		if data["message"] != fmt.Sprintf("%s%d", t.Name(), i) {
			t.Fatalf("Unexpected msg : %s\n", string(msg))
		}
	}
}
