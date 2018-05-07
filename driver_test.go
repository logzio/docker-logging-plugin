package main

import(
	"bytes"
	"bufio"
	"encoding/json"
	"fmt"
	"testing"
	"time"
	"github.com/beeker1121/goque"
	"github.com/docker/docker/daemon/logger"
	"net/http"
	"os"

	"net/http/httptest"

	"github.com/docker/docker/api/types/backend"
)

func TestValidateDriverOpt(t *testing.T){//TODO - update cases
	conf := map[string]string{
		logzioFormat: 		"json",
		logzioLogSource: 	"logzioSource",
		logzioTag: 			"logzioTag",
		logzioToken:		"logzioToken",
		logzioType:			"logzioType",
		logzioUrl:			"logzioUrl",
		logzioDirPath:		fmt.Sprintf("./%s", t.Name()),
		envRegex:			"reg",
		dockerLabels:		"label",
		dockerEnv:			"env",
	}

	info := logger.Info{
		ContainerID: "123456789",
		Config: 	 conf,
	}

	if _, err := validateDriverOpt(info); err != nil{
		t.Fatalf("TestValidDriverOpt: %s", err)
	}
}


func TestMissingToken(t *testing.T){
	conf := map[string]string{
		logzioUrl:		"logzioUrl",
		logzioDirPath:	fmt.Sprintf("./%s", t.Name()),
	}

	info := logger.Info{
		ContainerID: "123456789",
		Config: 	 conf,
	}

	if _, err := validateDriverOpt(info); err.Error() != "logz.io token is required" {
		t.Fatalf("Failed TestMissingToken, got wrong error: %s", err)
	}
}


func TestNoSuchLogOpt(t *testing.T){
	conf := map[string]string{
		logzioFormat: 		"json",
		logzioLogSource:	"logzioSource",
		logzioTag: 			"logzioTag",
		logzioToken:		"logzioToken",
		logzioType:			"logzioType",
		logzioUrl:			"logzioUrl",
		logzioDirPath:		fmt.Sprintf("./%s", t.Name()),
		"logzioDummy":		"logzioDummy",
	}

	info := logger.Info{
		ContainerID: "123456789",
		Config: 	 conf,
	}

	if _, err := validateDriverOpt(info); err.Error() != "wrong log-opt: 'logzioDummy' - 123456789"{
		t.Fatalf("Failed TestNoSuchLogOpt got wrong error: %s", err)
	}
}

func TestSendingString(t *testing.T){
	mock := NewtestHTTPMock(t, []int{http.StatusOK, http.StatusOK})
	go mock.Serve()
	defer mock.Close()
	info := logger.Info{
		Config: map[string]string{
			logzioUrl:   	mock.URL(),
			logzioToken:	mock.Token(),
			logzioFormat: 	defaultFormat,
			logzioDirPath:	fmt.Sprintf("./%s", t.Name()),
			logzioTag:		"{{.ImageName}}",
			dockerLabels:	"labelKey",
			envRegex:		"^envKey",
		},
		ContainerID:        "containeriid",
		ContainerName:      "/container_name",
		ContainerImageID:   "contaimageid",
		ContainerImageName: "container_image_name",
		ContainerEnv:		[]string{"envKey=envValue"},
		ContainerLabels: 	map[string]string{
			"labelKey": "labelValue",
		},
	}

	logziol, err := newLogzioLogger(info, nil, "0")
	if err != nil{
		t.Fatal(err)
	}
	defer os.RemoveAll(info.Config[logzioDirPath])
	plmd := &backend.PartialLogMetaData{
		Last:	true,
		ID:		info.ContainerID,
	}
	str := &logger.Message{
		Line: 		[]byte("string"),
		Source:  	"stdout",
		Timestamp: 	time.Now(),
		PLogMetaData: plmd,
	}
	if err := logziol.Log(str); err != nil{
		t.Fatalf("Failed Log string: %s", err)
	}

	err = logziol.Close()
	if err != nil{
		t.Fatal(err)
	}

	hostname, err := info.Hostname()
	if err != nil{
		t.Fatal(err)
	}
	// check string message
	sm := mock.messages[0]
	if sm.Host != hostname ||
		sm.LogSource != "stdout" ||
		sm.Tags != info.ContainerID ||
		sm.Type != defaultSourceType ||
		sm.Time != time.Unix(0, str.Timestamp.UnixNano()).Format(time.RFC3339Nano){
		t.Fatalf("Failed string message, one of the meata data is missing. %+v\n", sm)
	}
	if sm.Extra["envKey"] != "envValue" || sm.Extra["labelKey"] != "labelValue"{
		t.Fatalf("Failed string message, one of the Extra fields is wrong. %+v\n", sm.Extra)
	}

	if sm.Message != "string"{
		t.Fatalf("Failed string message, not a string: %s", sm.Message)
	}
}

func TestSendingJson(t *testing.T){
	mock := NewtestHTTPMock(t, []int{http.StatusOK, http.StatusOK})
	go mock.Serve()
	defer mock.Close()
	info := logger.Info{
		Config: map[string]string{
			logzioUrl:   	mock.URL(),
			logzioToken:	mock.Token(),
			logzioFormat: 	jsonFormat,
			logzioDirPath:	fmt.Sprintf("./%s", t.Name()),
			logzioTag:		"{{.ImageName}}",
			dockerLabels:	"labelKey",
			envRegex:		"^envKey",
		},
		ContainerID:        "containeriid",
		ContainerName:      "/container_name",
		ContainerImageID:   "contaimageid",
		ContainerImageName: "container_image_name",
		ContainerEnv:		[]string{"envKey=envValue"},
		ContainerLabels: 	map[string]string{
			"labelKey": "labelValue",
		},
	}

	logziol, err := newLogzioLogger(info, nil, "0")
	if err != nil{
		t.Fatal(err)
	}
	defer os.RemoveAll(info.Config[logzioDirPath])
	plmd := &backend.PartialLogMetaData{
		Last:	true,
		ID:		info.ContainerID,
	}

	jstr := &logger.Message{
		Line: 			[]byte("{\"key\":\"value\"}"),
		Source:  		"stdout",
		Timestamp: 		time.Now(),
		PLogMetaData:	plmd,
	}

	if err := logziol.Log(jstr); err != nil{
		t.Fatalf("Failed Log json: %s", err)
	}

	err = logziol.Close()
	if err != nil{
		t.Fatal(err)
	}

	hostname, err := info.Hostname()
	if err != nil{
		t.Fatal(err)
	}
	// check json message
	jm := mock.messages[0]
	if jm.Host != hostname ||
		jm.LogSource != "stdout" ||
		jm.Tags != info.ContainerID ||
		jm.Type != defaultSourceType ||
		jm.Time != time.Unix(0, jstr.Timestamp.UnixNano()).Format(time.RFC3339Nano){
		t.Fatalf("Failed json message, one of the meata data is missing. %+v\n", jm)
	}

	if jm.Extra["envKey"] != "envValue" || jm.Extra["labelKey"] != "labelValue"{
		t.Fatalf("Failed string message, one of the Extra fields is wrong. %+v\n", jm.Extra)
	}

	// casting message to a map
	if jm.Message.(map[string]interface{})["key"] != "value"{
		t.Fatalf("Failed json message, not a json: %v", jm.Message)
	}
}

func TestSendingNoTag(t *testing.T) {
	mock := NewtestHTTPMock(t, []int{http.StatusOK, http.StatusOK})
	go mock.Serve()
	defer mock.Close()
	info := logger.Info{
		Config: map[string]string{
			logzioUrl:     mock.URL(),
			logzioToken:   mock.Token(),
			logzioFormat:  jsonFormat,
			logzioDirPath: fmt.Sprintf("./%s", t.Name()),
			logzioTag:		"",
		},
		ContainerID:        "containeriid",
		ContainerName:      "/container_name",
		ContainerImageID:   "contaimageid",
		ContainerImageName: "container_image_name",
	}
	logziol, err := newLogzioLogger(info, nil, "0")
	if err != nil{
		t.Fatal(err)
	}
	defer os.RemoveAll(info.Config[logzioDirPath])
	plmd := &backend.PartialLogMetaData{
		Last:	true,
		ID:		info.ContainerID,
	}
	str := &logger.Message{
		Line:      		[]byte("string"),
		Source:    		"stdout",
		Timestamp: 		time.Now(),
		PLogMetaData: 	plmd,
	}

	if err := logziol.Log(str); err != nil {
		t.Fatalf("Failed Log string: %s", err)
	}
	err = logziol.Close()
	if err != nil{
		t.Fatal(err)
	}

	hostname, err := info.Hostname()
	if err != nil{
		t.Fatal(err)
	}
	// check string message
	sm := mock.messages[0]
	if sm.Host != hostname ||
		sm.LogSource != "stdout" ||
		sm.Tags != "" ||
		sm.Type != defaultSourceType ||
		sm.Extra != nil ||
		sm.Time != time.Unix(0, str.Timestamp.UnixNano()).Format(time.RFC3339Nano){
		t.Fatalf("Failed string message, one of the meata data is missing. %+v\n", sm)
	}
	if sm.Message != "string"{
		t.Fatalf("Failed string message, not a string: %s", sm.Message)
	}
}

func TestTimerSendingNotExpired(t *testing.T){
	mock := NewtestHTTPMock(t, []int{http.StatusOK, http.StatusOK})
	go mock.Serve()
	defer mock.Close()

	info := logger.Info{
		Config: map[string]string{
			logzioUrl:   	mock.URL(),
			logzioToken:	mock.Token(),
			logzioFormat: 	jsonFormat,
			logzioDirPath:	fmt.Sprintf("./%s", t.Name()),
		},
		ContainerID:        "containeriid",
		ContainerName:      "/container_name",
		ContainerImageID:   "contaimageid",
		ContainerImageName: "container_image_name",
	}
	logziol, err := newLogzioLogger(info, nil, "0")
	if err != nil{
		t.Fatal(err)
	}
	defer os.RemoveAll(info.Config[logzioDirPath])
	plmd := &backend.PartialLogMetaData{
		Last:	true,
		ID:		info.ContainerID,
	}
	msgTime := time.Now()
	str := &logger.Message{
		Line: 			[]byte("string"),
		Source:  		"stdout",
		Timestamp: 		msgTime,
		PLogMetaData:	plmd,
	}

	if err := logziol.Log(str); err != nil{
		t.Fatalf("Failed Log string: %s", err)
	}

	<- time.After(defaultLogsDrainTimeout - (time.Second * 1))

	plmd = &backend.PartialLogMetaData{
		Last:	true,
		ID:		info.ContainerID,
	}

	str = &logger.Message{
		Line: 			[]byte("string"),
		Source:  		"stdout",
		Timestamp: 		msgTime,
		PLogMetaData:	plmd,
	}

	if err := logziol.Log(str); err != nil{
		t.Fatalf("Failed Log string: %s", err)
	}

	err = logziol.Close()
	if err != nil{
		t.Fatal(err)
	}

	<- time.After(250 * time.Millisecond)

	batchNumber := mock.Batch()
	if batchNumber != 1{
		t.Fatalf("Unexpected batch number %d. Expected 1", batchNumber)
	}
	// sanity check
	firstMsg, secondMsg := mock.messages[0], mock.messages[1]
	if firstMsg.Message != secondMsg.Message ||
		firstMsg.LogSource != secondMsg.LogSource ||
		firstMsg.Host != secondMsg.Host||
		firstMsg.Time != secondMsg.Time ||
		firstMsg.Tags != secondMsg.Tags{
		t.Fatal("Expecting messages to be the same")
	}
}

func TestTimerSendingExpired(t *testing.T){
	mock := NewtestHTTPMock(t, []int{http.StatusOK, http.StatusOK})
	go mock.Serve()
	defer mock.Close()

	info := logger.Info{
		Config: map[string]string{
			logzioUrl:   	mock.URL(),
			logzioToken:	mock.Token(),
			logzioFormat: 	jsonFormat,
			logzioDirPath:	fmt.Sprintf("./%s", t.Name()),
		},
		ContainerID:        "containeriid",
		ContainerName:      "/container_name",
		ContainerImageID:   "contaimageid",
		ContainerImageName: "container_image_name",
	}
	logziol, err := newLogzioLogger(info, nil, "0")
	if err != nil{
		t.Fatal(err)
	}
	defer os.RemoveAll(info.Config[logzioDirPath])
	plmd := &backend.PartialLogMetaData{
		Last:	true,
		ID:		info.ContainerID,
	}
	msgTime := time.Now()
	str := &logger.Message{
		Line: 			[]byte("string"),
		Source:  		"stdout",
		Timestamp: 		msgTime,
		PLogMetaData:	plmd,
	}

	if err := logziol.Log(str); err != nil{
		t.Fatalf("Failed Log string: %s", err)
	}

	<- time.After(defaultLogsDrainTimeout + (time.Second * 1))
	plmd = &backend.PartialLogMetaData{
		Last:	true,
		ID:		info.ContainerID,
	}
	str = &logger.Message{
		Line: 			[]byte("string"),
		Source:  		"stdout",
		Timestamp: 		msgTime,
		PLogMetaData:	plmd,
	}

	if err := logziol.Log(str); err != nil{
		t.Fatalf("Failed Log string: %s", err)
	}

	err = logziol.Close()
	if err != nil{
		t.Fatal(err)
	}

	<- time.After(250 * time.Millisecond)

	batchNumber := mock.Batch()
	if batchNumber != 2{
		t.Fatalf("Unexpected batch number %d. Expected 1", batchNumber)
	}
	// sanity check
	sMsg, jMsg := mock.messages[0], mock.messages[1]
	if sMsg.Message != jMsg.Message ||
		sMsg.LogSource != jMsg.LogSource ||
		sMsg.Host != jMsg.Host||
		sMsg.Time != jMsg.Time ||
		sMsg.Tags != jMsg.Tags{
		t.Fatal("Expecting messages to be the same")
	}
}

func TestPartialSendingString(t *testing.T){
	mock := NewtestHTTPMock(t, []int{http.StatusOK, http.StatusOK})
	go mock.Serve()
	defer mock.Close()

	info := logger.Info{
		Config: map[string]string{
			logzioUrl:   	mock.URL(),
			logzioToken:	mock.Token(),
			logzioFormat: 	jsonFormat,
			logzioDirPath:	fmt.Sprintf("./%s", t.Name()),
		},
		ContainerID:        "containeriid",
		ContainerName:      "/container_name",
		ContainerImageID:   "contaimageid",
		ContainerImageName: "container_image_name",
	}
	logziol, err := newLogzioLogger(info, nil, "0")
	if err != nil{
		t.Fatal(err)
	}
	defer os.RemoveAll(info.Config[logzioDirPath])
	plmd := &backend.PartialLogMetaData{
		Last:	false,
		ID:		info.ContainerID,
	}
	msgTime := time.Now()
	str := &logger.Message{
		Line: 			[]byte("str"),
		Source:  		"stdout",
		Timestamp: 		msgTime,
		PLogMetaData:	plmd,
	}

	if err := logziol.Log(str); err != nil{
		t.Fatalf("Failed Log string: %s", err)
	}

	plmd = &backend.PartialLogMetaData{
		Last:	true,
		ID:		info.ContainerID,
	}
	msgTime = time.Now()
	str = &logger.Message{
		Line: 			[]byte("ing"),
		Source:  		"stdout",
		Timestamp: 		msgTime,
		PLogMetaData:	plmd,
	}

	if err := logziol.Log(str); err != nil{
		t.Fatalf("Failed Log string: %s", err)
	}

	err = logziol.Close()
	if err != nil{
		t.Fatal(err)
	}

	<- time.After(250 * time.Millisecond)

	batchNumber := mock.Batch()
	if batchNumber != 1{
		t.Fatalf("Unexpected batch number %d. Expected 1", batchNumber)
	}
	hostname, err := os.Hostname()
	if err != nil{
		t.Fatal(err)
	}
	// sanity check
	sMsg := mock.messages[0]
	if sMsg.Message != "string" ||
		sMsg.LogSource != "stdout" ||
		sMsg.Host != hostname||
		sMsg.Time != time.Unix(0, str.Timestamp.UnixNano()).Format(time.RFC3339Nano) ||
		sMsg.Tags != info.ContainerID{
			t.Fatal("Expecting messages to be the same")
	}
}

func TestPartialSendingJson(t *testing.T){
	mock := NewtestHTTPMock(t, []int{http.StatusOK, http.StatusOK})
	go mock.Serve()
	defer mock.Close()

	info := logger.Info{
		Config: map[string]string{
			logzioUrl:   	mock.URL(),
			logzioToken:	mock.Token(),
			logzioFormat: 	jsonFormat,
			logzioDirPath:	fmt.Sprintf("./%s", t.Name()),
			logzioType:		"type",
		},
		ContainerID:        "containeriid",
		ContainerName:      "/container_name",
		ContainerImageID:   "contaimageid",
		ContainerImageName: "container_image_name",
	}
	logziol, err := newLogzioLogger(info, nil, "0")
	if err != nil{
		t.Fatal(err)
	}
	defer os.RemoveAll(info.Config[logzioDirPath])
	plmd := &backend.PartialLogMetaData{
		Last:	false,
		ID:		info.ContainerID,
	}
	msgTime := time.Now()
	jstr := &logger.Message{
		Line: 			[]byte("{\"key\""),
		Source:  		"stdout",
		Timestamp: 		msgTime,
		PLogMetaData:	plmd,
	}

	if err := logziol.Log(jstr); err != nil{
		t.Fatalf("Failed Log string: %s", err)
	}

	plmd = &backend.PartialLogMetaData{
		Last:	true,
		ID:		info.ContainerID,
	}
	msgTime = time.Now()
	jstr = &logger.Message{
		Line: 			[]byte(":\"value\"}"),
		Source:  		"stdout",
		Timestamp: 		msgTime,
		PLogMetaData:	plmd,
	}

	if err := logziol.Log(jstr); err != nil{
		t.Fatalf("Failed Log string: %s", err)
	}

	err = logziol.Close()
	if err != nil{
		t.Fatal(err)
	}

	<- time.After(250 * time.Millisecond)

	batchNumber := mock.Batch()
	if batchNumber != 1{
		t.Fatalf("Unexpected batch number %d. Expected 1", batchNumber)
	}
	hostname, err := info.Hostname()
	if err != nil{
		t.Fatal(err)
	}
	// check json message
	jm := mock.messages[0]
	if jm.Host != hostname ||
		jm.LogSource != "stdout" ||
		jm.Tags != info.ContainerID ||
		jm.Type != "type" ||
		jm.Time != time.Unix(0, jstr.Timestamp.UnixNano()).Format(time.RFC3339Nano){
		t.Fatalf("Failed json message, one of the meata data is missing. %+v\n", jm)
	}

	// casting message to a map
	if jm.Message.(map[string]interface{})["key"] != "value"{
		t.Fatalf("Failed json message, not a json: %v", jm.Message)
	}
}

func TestDrainAfterClosed(t *testing.T){
	resp := []int{http.StatusInternalServerError, http.StatusInternalServerError, http.StatusOK}
	mock := NewtestHTTPMock(t, resp)
	go mock.Serve()
	defer mock.Close()
	info := logger.Info{
		Config: map[string]string{
			logzioUrl:   	mock.URL(),
			logzioToken:	mock.Token(),
			logzioFormat: 	jsonFormat,
			logzioDirPath:	fmt.Sprintf("./%s", t.Name()),
		},
		ContainerID:        "containeriid",
		ContainerName:      "/container_name",
		ContainerImageID:   "contaimageid",
		ContainerImageName: "container_image_name",
	}
	if err := os.Setenv(envLogsDrainTimeout, "60s"); err != nil{
		t.Fatal(err)
	}

	logziol, err := newLogzioLogger(info, nil, "0")
	if err != nil{
		t.Fatal(err)
	}
	defer os.RemoveAll(info.Config[logzioDirPath])
	plmd := &backend.PartialLogMetaData{
		Last:	true,
		ID:		info.ContainerID,
	}
	for i:=0; i<5; i++{
		if err := logziol.Log(&logger.Message{Line: []byte(fmt.Sprintf("%s%d", "str", i)), Source: "stdout",
		Timestamp: time.Now(), PLogMetaData: plmd}); err != nil{
			t.Fatalf("Failed Log string: %s", err)
		}
	}

	err = logziol.Close()
	if err != nil{
		t.Fatal(err)
	}

	batchNumber := mock.Batch()
	if batchNumber != len(resp){
		t.Fatalf("Unexpected batch number %d. Expected %d retries", batchNumber, 3)
	}

	// sanity check
	for i:=0; i<5; i++{
		if mock.messages[i].Message != fmt.Sprintf("%s%d", "str", i){
			t.Fatalf("message %g not matching message %d", mock.messages[i].Message, i)
		}
	}
}

func TestServerIsDown(t *testing.T){
	var sent= make([]byte, 1024)
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		r.Body.Read(sent)
	}))
	defer ts.Close()
	info := logger.Info{
		Config: map[string]string{
			logzioUrl:      "http://localhost:12345",
			logzioToken:    "123456789",
			logzioFormat: 	jsonFormat,
			logzioDirPath:	fmt.Sprintf("./%s", t.Name()),
		},
		ContainerID:        "containeriid",
		ContainerName:      "/container_name",
		ContainerImageID:   "contaimageid",
		ContainerImageName: "container_image_name",
	}
	logziol, err := newLogzioLogger(info, nil, "0")
	if err != nil{
		t.Fatal(err)
	}

	defer os.RemoveAll(info.Config[logzioDirPath])
	plmd := &backend.PartialLogMetaData{
		Last:	true,
		ID:		info.ContainerID,
	}
	for i:=0; i<5; i++{
		if err := logziol.Log(&logger.Message{Line: []byte(fmt.Sprintf("%s%d", t.Name(), i)),
		Source: "stdout", Timestamp: time.Now(), PLogMetaData: plmd}); err != nil{
			t.Fatalf("Failed Log string: %s", err)
		}
	}

	err = logziol.Close()
	if err != nil{
		t.Fatal(err)
	}

	//check the logs are saved to disk
	q, err := goque.OpenQueue(fmt.Sprintf("./%s/0", t.Name()))
	//We requeue as one big string
	if q.Length() != 1{
		t.Fatalf("Queue length is not as expected: %d", q.Length())
	}
	item, errQ := q.Dequeue()
	if errQ != nil {
		t.Fatal(errQ)
	}
	bytesReader := bytes.NewBuffer(item.Value)
	bufReader := bufio.NewReader(bytesReader)
	for i:=0; i<5; i++{
		var data map[string]interface{}
		msg, _, _:= bufReader.ReadLine()
		err := json.Unmarshal([]byte(msg), &data)
		if err != nil {
			panic(err)
		}
		if data["message"] != fmt.Sprintf("%s%d", t.Name(), i){
			t.Fatalf("Unexpected msg : %s\n", string(msg))
		}
	}
}


