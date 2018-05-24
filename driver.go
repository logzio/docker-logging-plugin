package main

import (
	"bytes"
	"context"
	"crypto/sha1"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"encoding/binary"
	"encoding/hex"
	"encoding/json"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/api/types/backend"
	"github.com/docker/docker/api/types/plugins/logdriver"
	"github.com/docker/docker/daemon/logger"
	"github.com/docker/docker/daemon/logger/jsonfilelog"
	"github.com/docker/docker/daemon/logger/loggerutils"
	"github.com/dougEfresh/logzio-go"
	"github.com/fatih/structs"
	"github.com/pkg/errors"
	"github.com/tonistiigi/fifo"

	protoio "github.com/gogo/protobuf/io"
)

const (
	//log-opt
	logzioFormat    = "logzio-format"
	logzioTag       = "logzio-tag"
	logzioToken     = "logzio-token"
	logzioType      = "logzio-type"
	logzioURL       = "logzio-url"
	logzioDirPath   = "logzio-dir-path"
	logzioLogSource = "logzio-source"
	logzioLogAttr   = "logzio-attributes"

	envLogsDrainTimeout           = "LOGZIO_DRIVER_LOGS_DRAIN_TIMEOUT"
	envChannelSize                = "LOGZIO_DRIVER_CHANNEL_SIZE"
	envDiskThreshold              = "LOGZIO_DRIVER_DISK_THRESHOLD"
	envMaxMsgBufferSize           = "LOGZIO_MAX_MSG_BUFFER_SIZE"
	envPartialBufferTimerDuration = "LOGZIO_MAX_PARTIAL_BUFFER_DURATION"

	envRegex     = "env-regex"
	dockerLabels = "labels"
	dockerEnv    = "env"

	defaultMaxMsgBufferSize           = 1024 * 1024
	defaultLogsDrainTimeout           = time.Second * 5
	defaultDiskThreshould             = 70
	defaultStreamChannelSize          = 10 * 1000
	defaultPartialBufferTimerDuration = time.Millisecond * 500
	defaultFlushPartialBuffer         = time.Second * 5

	defaultFormat     = "text"
	driverName        = "logzio"
	defaultSourceType = "logzio-docker-driver"
	jsonFormat        = "json"
)

type Driver struct {
	idx     map[string]*ContainerLoggersCtx
	logger  logger.Logger
	logs    map[string]*ContainerLoggersCtx
	mu      sync.Mutex // Protecting concurrency access for driver's maps
	senders map[string]*SenderConfigurations
}

type ContainerLoggersCtx struct {
	info         logger.Info
	jsonLogger   logger.Logger
	logzioLogger LogzioLogger
	stream       io.ReadCloser
}

type LogzioMessage struct {
	Host      string      `structs:"hostname"`
	Message   interface{} `structs:"message"`
	LogSource string      `structs:"log_source,omitempty"`
	Tags      string      `structs:"tags,omitempty"`
	Time      string      `structs:"driver_timestamp"`
	Type      string      `structs:"type,omitempty"`
}

type LogzioLogger struct {
	logger.Logger
	closed            bool
	closedDriverCond  *sync.Cond
	logzioSender      *logzio.LogzioSender
	lock              sync.RWMutex
	logFormat         string
	maxMsgBufferSize  int
	msg               map[string]interface{}
	msgStream         chan map[string]interface{}
	partialBufTimeout time.Duration
	pBuf              *PartialBuffer
	url               string
}

type SenderConfigurations struct {
	info     logger.Info
	hashCode string
	sender   *logzio.LogzioSender
}

func newDriver() *Driver {
	driver := &Driver{
		logs:    make(map[string]*ContainerLoggersCtx),
		idx:     make(map[string]*ContainerLoggersCtx),
		senders: make(map[string]*SenderConfigurations),
	}
	go driver.flushPartialBuffers()
	return driver
}

func (d *Driver) flushPartialBuffers() {
	timeout := defaultFlushPartialBuffer
	for {
		for containerID, containerLoggerInfo := range d.idx {
			d.mu.Lock()
			if _, ok := d.idx[containerID]; !ok {
				d.mu.Unlock()
				continue
			}
			logzioLogger := containerLoggerInfo.logzioLogger
			delta := time.Now().Sub(logzioLogger.pBuf.startTime)
			if delta > logzioLogger.pBuf.timeout {
				var msg logger.Message
				msg.Line = logzioLogger.pBuf.buf
				msg.Source = logzioLogger.pBuf.source
				msg.Timestamp = time.Unix(0, logzioLogger.pBuf.timeNano)
				msg.PLogMetaData = &backend.PartialLogMetaData{
					Last: true,
					ID:   containerID,
				}

				if err := logzioLogger.Log(&msg); err != nil {
					logrus.WithField("id", containerID).WithError(err).WithField("message", msg).
						Error("Logz.io logger:error writing log message")
				}
			}
			d.mu.Unlock()
		}
		time.Sleep(timeout)
	}
}

func validateDriverOpt(loggerInfo logger.Info) (string, error) {
	config := loggerInfo.Config
	// Config in logger.info is map[string]string
	for opt := range config {
		switch opt {
		case logzioFormat, logzioLogSource, logzioTag, logzioToken, logzioType, logzioURL, logzioDirPath,
			envRegex, dockerLabels, dockerEnv, logzioLogAttr:
		default:
			return "", fmt.Errorf("wrong log-opt: '%s' - %s\n", opt, loggerInfo.ContainerID)
		}
	}
	_, ok := config[logzioDirPath]
	if !ok {
		return "", fmt.Errorf("logz.io dir path is required. config: %v+\n", config)
	}

	token, ok := config[logzioToken]
	if !ok {
		return "", fmt.Errorf("logz.io token is required\n")
	}

	hashCode := hash(token, config[logzioDirPath])

	return hashCode, nil
}

func getTags(loggerInfo logger.Info) (string, error) {
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

func getHostname(loggerInfo logger.Info) (string, error) {
	// https://github.com/moby/moby/blob/master/daemon/logger/loginfo.go
	hostname, err := loggerInfo.Hostname()
	if err != nil {
		return "", fmt.Errorf("%s: cannot access hostname to set source field\n", driverName)
	}
	return hostname, nil
}

func getExtras(loggerInfo logger.Info) (map[string]string, error) {
	// https://github.com/moby/moby/blob/master/daemon/logger/loginfo.go
	extra, err := loggerInfo.ExtraAttributes(nil)
	if err != nil {
		return nil, err
	}
	return extra, nil
}

func getAttributes(loggerInfo logger.Info) map[string]interface{} {
	attrMap := make(map[string]interface{})
	attrStr, ok := loggerInfo.Config[logzioLogAttr]
	if !ok {
		return nil
	}
	err := json.Unmarshal([]byte(attrStr), &attrMap)
	if err != nil {
		logrus.Info("Failed to extract log attributes, please verify the format is correct\n")
		return nil
	}
	return attrMap
}

func getFormat(loggerInfo logger.Info) string {
	format, ok := loggerInfo.Config[logzioFormat]
	if !ok {
		format = defaultFormat
	}

	if format == defaultFormat || format == jsonFormat {
		return format
	}
	logrus.Error(fmt.Sprintf("%s is not part of the format options we support: %s, json\n", format, defaultFormat))
	logrus.Info(fmt.Sprintf("Using default format instead: %s\n", defaultFormat))
	return defaultFormat
}

func getEnvInt(env string, dValue int) int {
	eValue := os.Getenv(env)
	if eValue == "" {
		return dValue
	}
	retVal, err := strconv.ParseInt(eValue, 10, 32)
	if err != nil {
		logrus.Error(fmt.Sprintf("Error parsing %s timeout %s\n", env, err))
		logrus.Info(fmt.Sprintf("Using default %s timeout %v\n", env, defaultLogsDrainTimeout))
		return dValue
	}
	return int(retVal)
}

func getEnvDuration(env string, dValue time.Duration) time.Duration {
	// Getenv retrieves the value of the environment variable named by the key.
	// It returns the value, which will be empty if the variable is not present.
	eDuration := os.Getenv(env)
	retDuration := defaultLogsDrainTimeout
	if eDuration != "" {
		var err error
		retDuration, err = time.ParseDuration(eDuration)
		if err != nil {
			logrus.Error(fmt.Sprintf("Error parsing drain timeout %s\n", err))
			logrus.Info(fmt.Sprintf("Using default drain timeout %v\n", dValue))
			retDuration = defaultLogsDrainTimeout
		}
	}
	return retDuration
}

func newLogzioSender(loggerInfo logger.Info, token string, sender *logzio.LogzioSender, hashCode string) (*logzio.LogzioSender, error) {
	if sender != nil {
		return sender, nil
	}
	drainDuration := getEnvDuration(envLogsDrainTimeout, defaultLogsDrainTimeout)
	urlStr, _ := loggerInfo.Config[logzioURL]
	dir, _ := loggerInfo.Config[logzioDirPath]
	eDiskThreshold := getEnvInt(envDiskThreshold, defaultDiskThreshould)
	lsender, err := logzio.New(token,
		logzio.SetDebug(os.Stderr),
		logzio.SetUrl(urlStr),
		logzio.SetDrainDiskThreshold(eDiskThreshold),
		logzio.SetTempDirectory(fmt.Sprintf("%s%s%s", dir, string(os.PathSeparator), hashCode)),
		logzio.SetDrainDuration(drainDuration))
	logrus.Debugf("Creating new logger for container %s\n", loggerInfo.ContainerID)
	return lsender, err
}

func hash(args ...string) string {
	var toHash string
	for _, s := range args {
		toHash += s
	}
	h := sha1.New()
	h.Write([]byte(toHash))
	return hex.EncodeToString(h.Sum(nil))
}

func newLogzioLogger(loggerInfo logger.Info, sender *logzio.LogzioSender, hashCode string) (*LogzioLogger, error) {
	optToken := loggerInfo.Config[logzioToken]

	hostname, err := getHostname(loggerInfo)
	if err != nil {
		return nil, err
	}

	extra, err := getExtras(loggerInfo)
	if err != nil {
		return nil, err
	}

	tags, err := getTags(loggerInfo)
	if err != nil {
		return nil, err
	}

	format := getFormat(loggerInfo)

	attr := getAttributes(loggerInfo)

	sourceType, ok := loggerInfo.Config[logzioType]
	if !ok {
		sourceType = defaultSourceType
	}
	logSource := loggerInfo.Config[logzioLogSource]
	streamSize := getEnvInt(envChannelSize, defaultStreamChannelSize)
	maxMsgBufferSize := getEnvInt(envMaxMsgBufferSize, defaultMaxMsgBufferSize)
	partialBufferTimeout := getEnvDuration(envPartialBufferTimerDuration, defaultPartialBufferTimerDuration)
	defaultMsg := structs.Map(&LogzioMessage{
		Host:      hostname,
		LogSource: logSource,
		Type:      sourceType,
		Tags:      tags,
	})
	for key, value := range attr {
		defaultMsg[key] = value
	}

	for key, value := range extra {
		defaultMsg[key] = value
	}

	logzioSender, err := newLogzioSender(loggerInfo, optToken, sender, hashCode)
	if err != nil {
		return nil, err
	}

	logzioLogger := &LogzioLogger{
		logzioSender:      logzioSender,
		logFormat:         format,
		maxMsgBufferSize:  maxMsgBufferSize,
		msg:               defaultMsg,
		msgStream:         make(chan map[string]interface{}, streamSize),
		partialBufTimeout: partialBufferTimeout,
	}

	go logzioLogger.sendToLogzio()
	return logzioLogger, nil
}

func (logzioLogger *LogzioLogger) sendToLogzio() {
	for {
		msg, open := <-logzioLogger.msgStream
		if open {
			if data, err := json.Marshal(msg); err != nil {
				logrus.Error(fmt.Sprintf("Error marshalling json object: %s\n", err.Error()))
			} else if err := logzioLogger.logzioSender.Send(data); err != nil {
				logrus.Error(fmt.Sprintf("Error enqueue object: %s\n", err))
			}

		} else {
			logzioLogger.logzioSender.Stop()
			logzioLogger.lock.Lock()
			logzioLogger.logzioSender.CloseIdleConnections()
			logzioLogger.closed = true
			logzioLogger.closedDriverCond.Signal()
			// better to not use defer in a loop if possible
			logzioLogger.lock.Unlock()
			break
		}
	}
}

func (logzioLogger *LogzioLogger) sendMessageToChannel(msg map[string]interface{}) error {
	logzioLogger.lock.RLock()
	defer logzioLogger.lock.RUnlock()
	// if Driver is closed return error
	if logzioLogger.closedDriverCond != nil {
		return fmt.Errorf("can't send the log to the channel - Driver is closed\n")
	}
	logzioLogger.msgStream <- msg
	return nil
}

func (logzioLogger *LogzioLogger) Log(msg *logger.Message) error {
	logMessage := make(map[string]interface{})
	for index, element := range logzioLogger.msg {
		logMessage[index] = element
	}
	logMessage["driver_timestamp"] = time.Unix(0, msg.Timestamp.UnixNano()).Format(time.RFC3339Nano)
	logMessage["log_source"] = msg.Source
	format := logzioLogger.logFormat
	if format == defaultFormat {
		logMessage["message"] = string(msg.Line)
	} else {
		// use of RawMessage: http://goinbigdata.com/how-to-correctly-serialize-json-string-in-golang/
		var jsonLogLine json.RawMessage
		if err := json.Unmarshal(msg.Line, &jsonLogLine); err == nil {
			logMessage["message"] = &jsonLogLine
			logMessage["logzio_codec"] = "json"
		} else {
			// do not try to fight it
			logMessage["message"] = string(msg.Line)
		}
	}
	err := logzioLogger.sendMessageToChannel(logMessage)
	return err
}

func (logzioLogger *LogzioLogger) Close() error {
	logzioLogger.lock.Lock()
	defer logzioLogger.lock.Unlock()
	if logzioLogger.closedDriverCond == nil {
		logzioLogger.closedDriverCond = sync.NewCond(&logzioLogger.lock)
		close(logzioLogger.msgStream)
		for !logzioLogger.closed {
			logzioLogger.closedDriverCond.Wait()
		}
	}
	return nil
}

func (logzioLogger *LogzioLogger) Name() string {
	return driverName
}

func (d *Driver) checkHashCodeExists(hashCode string, token string) *logzio.LogzioSender {
	if _, ok := d.senders[token]; ok {
		if hashCode != d.senders[token].hashCode {
			logrus.Error(fmt.Sprintf("Can use only one configuration set per token: %+v\n", d.senders[token].info))
		}
		return d.senders[token].sender
	}
	sc := &SenderConfigurations{}
	d.senders[token] = sc
	return nil
}

func (d *Driver) StartLogging(file string, logCtx logger.Info) error {
	d.mu.Lock()
	if _, exists := d.logs[file]; exists {
		d.mu.Unlock()
		return fmt.Errorf("logger for %q already exists\n", file)
	}
	d.mu.Unlock()

	if logCtx.LogPath == "" {
		logCtx.LogPath = filepath.Join("/var/log/docker", logCtx.ContainerID)
	}

	if err := os.MkdirAll(filepath.Dir(logCtx.LogPath), 0755); err != nil {
		return errors.Wrapf(err, "error setting up logger dir\n")
	}

	jsonLogger, err := jsonfilelog.New(logCtx)
	if err != nil {
		return errors.Wrap(err, "error creating jsonfile logger\n")
	}

	logrus.WithField("id", logCtx.ContainerID).WithField("file", file).WithField("logpath", logCtx.LogPath).Debugf("Start logging")
	f, err := fifo.OpenFifo(context.Background(), file, syscall.O_RDONLY, 0700)
	if err != nil {
		return errors.Wrapf(err, "error opening logger fifo: %q\n", file)
	}

	hashCode, err := validateDriverOpt(logCtx)
	if err != nil {
		return errors.Wrap(err, "error in one of the logger options\n")
	}

	// notify the user if we are using previous configurations.
	sender := d.checkHashCodeExists(hashCode, logCtx.Config[logzioToken])
	logzioLogger, err := newLogzioLogger(logCtx, sender, hashCode)
	if err != nil {
		return errors.Wrap(err, "error creating logzio logger")
	}
	d.mu.Lock()
	lf := &ContainerLoggersCtx{logCtx, jsonLogger, *logzioLogger, f}
	d.logs[file] = lf
	d.idx[logCtx.ContainerID] = lf
	if sender == nil {
		d.senders[logCtx.Config[logzioToken]].sender = logzioLogger.logzioSender
		d.senders[logCtx.Config[logzioToken]].info = logCtx
		d.senders[logCtx.Config[logzioToken]].hashCode = hashCode
	}
	d.mu.Unlock()

	go consumeLog(lf)
	return nil
}

func (d *Driver) StopLogging(file string) error {
	logrus.WithField("file", file).Debugf("Stop logging")
	d.mu.Lock()
	lf, ok := d.logs[file]
	if ok {
		logrus.Info(fmt.Sprintf("%s: Stopping logging Driver for closed container %s.", driverName, lf.info.ContainerID))
		lf.stream.Close()
		lf.jsonLogger.Close()
		delete(d.logs, file)
	}
	d.mu.Unlock()
	return nil
}

func consumeLog(lf *ContainerLoggersCtx) {
	dec := protoio.NewUint32DelimitedReader(lf.stream, binary.BigEndian, 1e6)
	defer dec.Close()
	defer func() {
		lf.stream.Close()
		lf.jsonLogger.Close()
	}()
	pBuf := &PartialBuffer{
		startTime: time.Now(),
		timeout:   lf.logzioLogger.partialBufTimeout,
		maxBytes:  lf.logzioLogger.maxMsgBufferSize,
	}
	lf.logzioLogger.pBuf = pBuf
	var buf logdriver.LogEntry
	for {
		if err := dec.ReadMsg(&buf); err != nil {
			if err == io.EOF || err == os.ErrClosed || strings.Contains(err.Error(), "file already closed") {
				if len(pBuf.buf) != 0 {
					logrus.WithField("id", lf.info.ContainerID).WithError(err).
						Warningf("Could not finish sending partial message before closing: %s\n", string(pBuf.buf))
				}
				logrus.WithField("id", lf.info.ContainerID).WithError(err).Debug("shutting down log logger")
				return
			}
			dec = protoio.NewUint32DelimitedReader(lf.stream, binary.BigEndian, 1e6)
		}
		tBuf := bytes.Trim(buf.Line, "\x00")
		if len(tBuf) != 0 {
			pBuf.Add(buf)
			delta := time.Now().Sub(pBuf.startTime)
			if !buf.Partial || delta > pBuf.timeout {
				var msg logger.Message
				msg.Line = pBuf.buf
				msg.Source = buf.Source
				msg.Timestamp = time.Unix(0, buf.TimeNano)
				msg.PLogMetaData = &backend.PartialLogMetaData{
					Last: !buf.Partial,
					ID:   lf.info.ContainerID,
				}

				if err := lf.logzioLogger.Log(&msg); err != nil {
					logrus.WithField("id", lf.info.ContainerID).WithError(err).WithField("message", msg).
						Error("Logz.io logger:error writing log message")
				}
				if err := lf.jsonLogger.Log(&msg); err != nil {
					logrus.WithField("id", lf.info.ContainerID).WithError(err).WithField("message", msg).
						Error("json logger: error writing log message")
				}
				pBuf.Reset()
			}
		}
		buf.Reset()
	}
}

func (d *Driver) ReadLogs(info logger.Info, config logger.ReadConfig) (io.ReadCloser, error) {
	d.mu.Lock()
	lf, exists := d.idx[info.ContainerID]
	d.mu.Unlock()
	if !exists {
		return nil, fmt.Errorf("logger does not exist for %s\n", info.ContainerID)
	}

	r, w := io.Pipe()
	lr, ok := lf.jsonLogger.(logger.LogReader)
	if !ok {
		return nil, fmt.Errorf("logger does not support reading\n")
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
				buf.Partial = false
				if msg.PLogMetaData != nil {
					buf.Partial = !msg.PLogMetaData.Last
				}
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
