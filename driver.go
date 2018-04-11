package main


import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/docker/docker/api/types/plugins/logdriver"
	"github.com/docker/docker/daemon/logger"
	"github.com/docker/docker/daemon/logger/jsonfilelog"
    "github.com/docker/docker/daemon/logger/loggerutils"
	protoio "github.com/gogo/protobuf/io"
	"github.com/pkg/errors"
	"github.com/Sirupsen/logrus"
	"github.com/tonistiigi/fifo"
	//"github.com/dougEfresh/logzio-go"
	"crypto/sha1"
	"encoding/hex"
)


const(
    //log-opt
    logzioFormat                =   "logzio-format"
    logzioTag                   =   "logzio-tag"
    logzioToken                 =   "logzio-token"
    logzioType                  =   "logzio-type"
    logzioUrl                   =   "logzio-url"
    logzioDirPath				= 	"logzio-dir-path"
    logzioLogSource				= 	"logzio-source"

    envLogsDrainTimeout    		=   "LOGZIO_DRIVER_LOGS_DRAIN_TIMEOUT"
	envChannelSize				=   "LOGZIO_DRIVER_CHANNEL_SIZE"
	envDiskThreshold    		=   "LOGZIO_DRIVER_DISK_THRESHOLD"

	envRegex					= 	"env-regex"
	dockerLabels				=	"labels"
	dockerEnv					=	"env"


    defaultLogsDrainTimeout 	= 	time.Second * 5
    defaultDiskThreshould		= 	70
    defaultStreamChannelSize	= 	5 * 1000

    defaultFormat           	= 	"text"
    driverName              	= 	"logzio"
    jsonFormat              	= 	"json"
)


type driver struct {
	mu     	sync.Mutex
	logs  	map[string]*logPair
	idx    	map[string]*logPair
	senders	map[string]*senderConfigurations
	logger 	logger.Logger
}

type logPair struct {
	l          logger.Logger
    logziol    logger.Logger
	stream     io.ReadCloser
	info       logger.Info
}

type logzioMessage struct{
    Message     interface{}         `json:"message"`
    Host        string              `json:"hostname"`
    Type        string              `json:"type,omitempty"`
    LogSource   string              `json:"log_source,omitempty"`
    Time        string              `json:"@timestamp"`
    Tags        string              `json:"tags,omitempty"`
    Extra       map[string]string   `json:"extra,omitempty"`
}

type logzioLogger struct{
    closed             bool
    closedChannel      chan int
	closedDriverCond   *sync.Cond
    logzioSender       *LogzioSender //TODO - change to git
    lock               sync.RWMutex
    logFormat          string
    msg                *logzioMessage
	msgStream          chan *logzioMessage
    url                string
}

type senderConfigurations struct {
	sender 		*LogzioSender
	info   		logger.Info
	hashCode	string
}


func newDriver() *driver {
	return &driver{
		logs: 		make(map[string]*logPair),
		idx:  		make(map[string]*logPair),
		senders:	make(map[string]*senderConfigurations),
	}
}

func validateDriverOpt(loggerInfo logger.Info) (string, error){
    config := loggerInfo.Config
    // Config in logger.info is map[string]string
    for opt := range config{
        switch opt {
		case logzioFormat:
        case logzioLogSource:
        case logzioTag:
        case logzioToken:
        case logzioType:
        case logzioUrl:
		case logzioDirPath:
		case envRegex:
		case dockerLabels:
		case dockerEnv:
        default:
            return "", fmt.Errorf("wrong log-opt: '%s' - %s", opt, loggerInfo.ContainerID)
        }
    }

	_, ok := config[logzioDirPath]
	if !ok{
		return "", fmt.Errorf("logz.io dir path is required")
	}

    token, ok := config[logzioToken]
    if !ok{
        return "", fmt.Errorf("logz.io token is required")
    }
	hashCode := hash(token,
		config[logzioDirPath])

	return hashCode, nil
}

func setTag(loggerInfo logger.Info) (string, error){
	tag := ""
	var err error
	if tagTemplate, ok := loggerInfo.Config[logzioTag]; !ok || tagTemplate != "" {
		tag, err = loggerutils.ParseLogTag(loggerInfo, loggerutils.DefaultTemplate)
		if err != nil {
			return "", err
		}
	}
	return tag, nil
}


//func setClient(loggerInfo logger.Info) (*http.Client, *http.Transport){
//    tlsConfig := &tls.Config{InsecureSkipVerify: true}
//
//    // Using this example - https://github.com/jcbsmpsn/golang-https-example
//    //if caPath, ok := loggerInfo.Config[logzioCAPath]; ok{
//    //    caCert, err := ioutil.ReadFile(caPath)
//    //    if err != nil {
//    //        return nil, nil, err
//    //    }
//    //    caCertPool := x509.NewCertPool()
//    //    caCertPool.AppendCertsFromPEM(caCert)
//    //    tlsConfig.RootCAs = caCertPool
//    //}
//	//
//    //if caName, ok := loggerInfo.Config[logzioCAName]; ok{
//    //    tlsConfig.ServerName = caName
//    //}
//	//
//    //if skipTLSVerify, ok := loggerInfo.Config[logzioInsecureSkipVerify]; ok {
//		//skipTLSVerify, err := strconv.ParseBool(skipTLSVerify)
//		//if err != nil {
//		//	return nil, nil,err
//		//}
//		//tlsConfig.InsecureSkipVerify = skipTLSVerify
//	//}
//
//	transport := &http.Transport{
//		TLSClientConfig: tlsConfig,
//	}
//
//	client := &http.Client{
//		Transport: transport,
//	}
//
//	return client, transport
//}

func setHostname(loggerInfo logger.Info) (string, error){
    // https://github.com/moby/moby/blob/master/daemon/logger/loginfo.go
    hostname, err := loggerInfo.Hostname()
    if err != nil{
        return "", fmt.Errorf("%s: cannot access hostname to set source field", driverName)
    }
    return hostname, nil
}

func setExtras(loggerInfo logger.Info) (map[string]string, error){
    // https://github.com/moby/moby/blob/master/daemon/logger/loginfo.go
    extra, err := loggerInfo.ExtraAttributes(nil)
    if err != nil {
        return nil, err
    }
    return extra, nil
}

func setFormat(loggerInfo logger.Info) string {
	format, ok := loggerInfo.Config[logzioFormat]
	if !ok{
		format = defaultFormat
	}

	if format == defaultFormat || format == jsonFormat {
		return format
	}
	logrus.Error(fmt.Sprintf("%s is not part of the format options we support: %s, json", format, defaultFormat))
	logrus.Info(fmt.Sprintf("Using default format instead: %s", defaultFormat))
	return defaultFormat
}

func getEnvInt(env string, dValue int) int{
	eValue := os.Getenv(env)
	if eValue == "" {
		return dValue
	}
	retVal, err := strconv.ParseInt(eValue, 10, 32)
	if err != nil {
		logrus.Error(fmt.Sprintf("Error parsing %s timeout %s", env, err))
		logrus.Info(fmt.Sprintf("Using default %s timeout %v", env, defaultLogsDrainTimeout))
		return dValue
	}
	return int(retVal)
}

func newLogzioSender(loggerInfo logger.Info, token string, sender *LogzioSender, hashCode string) (*LogzioSender, error){//TODO - change to git
	if sender != nil{
		return sender, nil
	}
	// Getenv retrieves the value of the environment variable named by the key.
	// It returns the value, which will be empty if the variable is not present.
	eDuration := os.Getenv(envLogsDrainTimeout)
	drainDuration := defaultLogsDrainTimeout
	if eDuration != "" {
		var err error
		drainDuration, err = time.ParseDuration(eDuration)
		if err != nil {
			logrus.Error(fmt.Sprintf("Error parsing drain timeout %s", err))
			logrus.Info(fmt.Sprintf("Using default drain timeout %v", defaultLogsDrainTimeout))
			drainDuration = defaultLogsDrainTimeout
		}
	}
	urlStr, _ := loggerInfo.Config[logzioUrl]
	dir , _:= loggerInfo.Config[logzioDirPath]
	eDiskThreshold := getEnvInt(envDiskThreshold, defaultDiskThreshould)

	return New(token,//TODO - change to git
		SetUrl(urlStr),//TODO - change to git
		SetDrainDiskThreshold(eDiskThreshold),//TODO - change to git
		SetDrainDuration(drainDuration),//TODO - change to git
		SetTempDirectory(fmt.Sprintf("%s_%s",dir,hashCode)))//TODO - change to git
}

func hash(args ...string) string{
	var toHash string
	for _, s := range args{
		toHash += s
	}
	h := sha1.New()
	h.Write([]byte(toHash))
	return hex.EncodeToString(h.Sum(nil))
}

func newLogzioLogger(loggerInfo logger.Info, sender *LogzioSender, hashCode string) (logger.Logger, error){
	optToken := loggerInfo.Config[logzioToken]

	hostname, err := setHostname(loggerInfo)
	if err != nil{ return nil, err }

	extra, err := setExtras(loggerInfo)
	if err != nil { return nil, err }

	tags, err := setTag(loggerInfo)
    if err != nil { return nil, err }

    format := setFormat(loggerInfo)

    sourceType := loggerInfo.Config[logzioType]
	logSource := loggerInfo.Config[logzioLogSource]
    streamSize := getEnvInt(envChannelSize, defaultStreamChannelSize)

    defaultMsg := &logzioMessage{
        Host:       hostname,
		LogSource:	logSource,
        Type: 		sourceType,
        Tags:       tags,
        Extra:      extra,
    }

    logzioSender, err := newLogzioSender(loggerInfo, optToken, sender, hashCode)
    if err != nil {return nil, err}

    logziol := &logzioLogger{
        closedChannel:      make(chan int),
        logzioSender:		logzioSender,
        logFormat:          format,
        msg:                defaultMsg,
    	msgStream:          make(chan *logzioMessage, streamSize),
	}

    go logziol.sendToLogzio()
    return logziol, nil
}

func (logziol *logzioLogger) sendToLogzio(){
    for{
		msg, open := <-logziol.msgStream
		if open{
			if data, err := json.Marshal(msg); err != nil {
				logrus.Error(fmt.Sprintf("Error marshalling json object: %s\n", err.Error()))
			} else if err := logziol.logzioSender.Send(data); err != nil {
				logrus.Error(fmt.Sprintf("Error enqueue object: %s\n", err))
				}
		}else{
			logziol.logzioSender.Stop()
			logziol.lock.Lock()
			logziol.logzioSender.httpTransport.CloseIdleConnections()
			logziol.closed = true
			logziol.closedDriverCond.Signal()
			// better to not use defer in a loop if possible
			logziol.lock.Unlock()
			break
		}
	}
}

func (logziol *logzioLogger) sendMessageToChannel(msg *logzioMessage) error{
    logziol.lock.RLock()
    defer logziol.lock.RUnlock()
    // if driver is closed return error
    if logziol.closedDriverCond != nil{
        return fmt.Errorf("can't send the log to the channel - driver is closed")
    }
    logziol.msgStream <- msg
    return nil
}

func (logziol *logzioLogger) Log(msg *logger.Message) error{
    logMessage := *logziol.msg
    logMessage.Time = fmt.Sprintf("%f", float64(msg.Timestamp.UnixNano())/float64(time.Second))
    logMessage.LogSource = msg.Source

    format := logziol.logFormat
    if format == defaultFormat{
        logMessage.Message = string(msg.Line)
    }else{
        // use of RawMessage: http://goinbigdata.com/how-to-correctly-serialize-json-string-in-golang/
        var jsonLogLine json.RawMessage
    	if err := json.Unmarshal(msg.Line, &jsonLogLine); err == nil {
    		logMessage.Message = &jsonLogLine
    	} else {
    		// don't try to fight it
    		logMessage.Message = string(msg.Line)
    	}
    }
    //https://github.com/moby/moby/blob/master/daemon/logger/logger.go
    // NewMessage returns a new message from the message sync.Pool
    // PutMessage puts the specified message back n the message pool.
    // The message fields are reset before putting into the pool.
    logger.PutMessage(msg)
	err := logziol.sendMessageToChannel(&logMessage)
    return err
}


func (logziol *logzioLogger) Close() error {
	logziol.lock.Lock()
	defer logziol.lock.Unlock()
	if logziol.closedDriverCond == nil {
		logziol.closedDriverCond = sync.NewCond(&logziol.lock)
		close(logziol.msgStream)
		for !logziol.closed {
			logziol.closedDriverCond.Wait()
		}
	}
	return nil
}


func (logziol *logzioLogger) Name() string{
	return driverName
}

func (d *driver) checkHashCodeExists(hashCode string, token string) *LogzioSender{
	if _,ok := d.senders[token]; ok{
		if hashCode != d.senders[token].hashCode{
			logrus.Error(fmt.Sprintf("Can use only one configuration set per token: %+v\n", d.senders[hashCode].info))
		}
		return d.senders[hashCode].sender
	}
	return nil
}

func (d *driver) StartLogging(file string, logCtx logger.Info) error {
	d.mu.Lock()
	if _, exists := d.logs[file]; exists {
		d.mu.Unlock()
		return fmt.Errorf("logger for %q already exists", file)
	}
	d.mu.Unlock()

    hashCode, err := validateDriverOpt(logCtx)
    if err != nil{
        return errors.Wrap(err, "error in one of the logger options")
    }

	if logCtx.LogPath == "" {
		logCtx.LogPath = filepath.Join("/var/log/docker", logCtx.ContainerID)
	}

	if err := os.MkdirAll(filepath.Dir(logCtx.LogPath), 0755); err != nil {
		return errors.Wrapf(err, "error setting up logger dir")
	}

	l, err := jsonfilelog.New(logCtx)
	if err != nil {
		return errors.Wrap(err, "error creating jsonfile logger")
	}
	// notify the user if we are using previous configurations.
	sender := d.checkHashCodeExists(hashCode, logCtx.Config[logzioToken])
    logziol, err := newLogzioLogger(logCtx, sender, hashCode)
    if err != nil {
		return errors.Wrap(err, "error creating logzio logger")
	}

	logrus.WithField("id", logCtx.ContainerID).WithField("file", file).WithField("logpath", logCtx.LogPath).Debugf("Start logging")
	f, err := fifo.OpenFifo(context.Background(), file, syscall.O_RDONLY, 0700)
	if err != nil {
		return errors.Wrapf(err, "error opening logger fifo: %q", file)
	}

	d.mu.Lock()
	lf := &logPair{l, logziol, f, logCtx}
	d.logs[file] = lf
	d.idx[logCtx.ContainerID] = lf
	d.mu.Unlock()

	go consumeLog(lf)
	return nil
}


func (d *driver) StopLogging(file string) error {
	logrus.WithField("file", file).Debugf("Stop logging")
	d.mu.Lock()
	lf, ok := d.logs[file]
	if ok {
		lf.stream.Close()
		delete(d.logs, file)
	}
	d.mu.Unlock()
	return nil
}

func consumeLog(lf *logPair) {
	dec := protoio.NewUint32DelimitedReader(lf.stream, binary.BigEndian, 1e6)
	defer dec.Close()
	var buf logdriver.LogEntry
	for {
		if err := dec.ReadMsg(&buf); err != nil {
			if err == io.EOF {
				logrus.WithField("id", lf.info.ContainerID).WithError(err).Debug("shutting down log logger")
				lf.stream.Close()
				return
			}
			dec = protoio.NewUint32DelimitedReader(lf.stream, binary.BigEndian, 1e6)
		}
		var msg logger.Message
		msg.Line = buf.Line
		msg.Source = buf.Source
		msg.Partial = buf.Partial
		msg.Timestamp = time.Unix(0, buf.TimeNano)

		if err := lf.l.Log(&msg); err != nil {
			logrus.WithField("id", lf.info.ContainerID).WithError(err).WithField("message", msg).Error("error writing log message")
			continue
		}

        if err := lf.logziol.Log(&msg); err != nil {
			logrus.WithField("id", lf.info.ContainerID).WithError(err).WithField("message", msg).Error("error writing log message")
			continue
		}
		buf.Reset()
	}
}

func (d *driver) ReadLogs(info logger.Info, config logger.ReadConfig) (io.ReadCloser, error) {
	d.mu.Lock()
	lf, exists := d.idx[info.ContainerID]
	d.mu.Unlock()
	if !exists {
		return nil, fmt.Errorf("logger does not exist for %s", info.ContainerID)
	}

	r, w := io.Pipe()
	lr, ok := lf.l.(logger.LogReader)
	if !ok {
		return nil, fmt.Errorf("logger does not support reading")
	}

	go func() {
		watcher := lr.ReadLogs(config)

		enc := protoio.NewUint32DelimitedWriter(w, binary.BigEndian)
		defer enc.Close()
		defer watcher.Close()

		var buf logdriver.LogEntry
		for {
			select {
			case msg, ok := <-watcher.Msg:
				if !ok {
					w.Close()
					return
				}

				buf.Line = msg.Line
				buf.Partial = msg.Partial
				buf.TimeNano = msg.Timestamp.UnixNano()
				buf.Source = msg.Source

				if err := enc.WriteMsg(&buf); err != nil {
					w.CloseWithError(err)
					return
				}
			case err := <-watcher.Err:
				w.CloseWithError(err)
				return
			}

			buf.Reset()
		}
	}()

	return r, nil
}
