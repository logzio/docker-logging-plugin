package main

import (
	"time"
	"fmt"
	"regexp"
	"strconv"

	"github.com/docker/docker/daemon/logger"
)

const (
	defaultNegate 	 = true
	defaultMatch  	 = "after"
	defaultMaxLines  = 500
	defaultMaxBytes  = 400000
	defaultTimeout	 = 5 * time.Second
	defaultSeparator = " "
	defaultFlushPrt  = ""
)

type Multiline struct {
	match			string
	separator		string
	pattern			string
	negate			bool
	timeout			time.Duration
	startTime		time.Time
	flushPtr		string
	maxLines		int
	maxBytes     	int
	buf				[]byte
	numLines		int
	debug			bool
	last			*logger.Message
}

func NewMultiLine(multilineConfig map[string]string) *Multiline{
	var err error
	negate := defaultNegate
	if conNegate, ok := multilineConfig["negate"]; ok{
		negate, err = strconv.ParseBool(conNegate)
		if err != nil{
			fmt.Errorf("Error parsing timeout to a time format %s\n", err)
			return nil
		}
	}

	match := defaultMatch
	if conMatch, ok := multilineConfig["match"]; ok{
		match = conMatch
		if match != "after" || match != "before"{
			fmt.Errorf("Error parsing timeout to a time format %s\n", err)
			return nil
		}
	}

	maxLines := defaultMaxLines
	if conMaxLines, ok := multilineConfig["maxLines"]; ok{
		maxLines, err = strconv.Atoi(conMaxLines)
		if err != nil{
			fmt.Errorf("Error parsing timeout to a time format %s\n", err)
			return nil
		}
	}

	maxBytes := defaultMaxBytes
	if conMaxBytes, ok := multilineConfig["maxBytes"]; ok{
		maxBytes, err = strconv.Atoi(conMaxBytes)
		if err != nil{
			fmt.Errorf("Error parsing timeout to a time format %s\n", err)
			return nil
		}
	}

	timeout := defaultTimeout
	if conTimeout, ok := multilineConfig["timeout"]; ok{
		timeout, err = time.ParseDuration(conTimeout)
		if err != nil{
			fmt.Errorf("Error parsing timeout to a time format %s\n", err)
			return nil
		}
	}

	separator := defaultSeparator
	if conSeparator, ok := multilineConfig["separator"]; ok{
		separator = conSeparator
	}

	flushPtr := defaultFlushPrt
	if conMaxBytes, ok := multilineConfig["flushPtr"]; ok{
		maxBytes, err = strconv.Atoi(conMaxBytes)
		if err != nil{
			fmt.Errorf("Error parsing timeout to a time format %s\n", err)
			return nil
		}
	}
	return &Multiline{
		match:		match,
		separator:	separator,
		pattern: 	multilineConfig["pattern"],
		negate: 	negate,
		flushPtr: 	flushPtr,
		maxLines:	maxLines,
		maxBytes:   maxBytes,
		numLines:	0,
		debug:		false,
		timeout:	timeout,
	}
}

func (ml *Multiline) debugLog(str string) {
	if ml.debug{
		fmt.Println(str)
	}
}

func (ml *Multiline) setStartingTime(start time.Time){
	ml.startTime = start
}

func (ml *Multiline) SetDebug(debug bool){
	ml.debug = debug
}

func (ml *Multiline) Reset(){
	ml.buf = nil
	ml.numLines = 0
	ml.last = nil
	ml.startTime = time.Now()
}

func (ml *Multiline) Add(msg *logger.Message)  (logger.Message, bool){
	if ml.match == "after"{
		return ml.matchAfter(msg)
	}else{
		return ml.matchBefore(msg)
	}
}

func (ml *Multiline) getMatch(line []byte, ptr string) bool{
	regex, err := regexp.Compile(ptr)
	if err != nil {
		ml.debugLog("Failed to compile pattern")
		return false
	}
	match := regex.Match(line)
	if ml.negate{
		match = !match
	}
	return match
}

func (ml *Multiline)Flush() logger.Message{
	msg := *ml.last
	msg.Line = ml.buf
	ml.Reset()
	return msg
}

func (ml *Multiline) finalize() (logger.Message, bool){
	if ml.maxBytes <= len(ml.buf) || ml.maxLines == ml.numLines || time.Now().Sub(ml.startTime) > ml.timeout{
		return ml.Flush(), true
	}
	return logger.Message{}, false
}

func (ml *Multiline) matchAfter(msg *logger.Message) (logger.Message, bool) {
	line := msg.Line
	// first read
	if len(ml.buf) == 0 {
		ml.addLine(msg)
		return ml.finalize()
	}
	matches := ml.getMatch(line, ml.pattern)
	if matches {
		ml.addLine(msg)
		return ml.finalize()
	} else if ml.flushPtr != "" && ml.getMatch(line, ml.flushPtr) {
		ml.addLine(msg)
		return ml.Flush(), true
	}
	retMsg := ml.Flush()
	ml.addLine(msg)
	return retMsg, true
}

func (ml *Multiline) matchBefore(msg *logger.Message) (logger.Message, bool) {
	line := msg.Line
	matches := ml.getMatch(line, ml.pattern)
	if matches {
		if len(ml.buf) == 0{
			ml.addLine(msg)
			return ml.finalize()
		}else {
			retMsg := ml.Flush()
			ml.addLine(msg)
			return retMsg, true
		}
	} else if ml.flushPtr != "" && ml.getMatch(line, ml.flushPtr) {
		ml.addLine(msg)
		return ml.Flush(), true
	}
	if len(ml.buf) != 0{
		ml.addLine(msg)
		return ml.finalize()
	}
	return *msg, true
}

// return true if line was added and we reached the size limit
func (ml *Multiline) addLine(msg *logger.Message){
	line := msg.Line
	sz := len(ml.buf)
	addSeparator := sz > 0
	if addSeparator {
		sz += 1
	}
	//space left
	space := ml.maxBytes - sz

	// do we have enough space?
	maxBytesReached := space > 0
	maxLinesReached := ml.numLines < ml.maxLines
	if maxBytesReached && maxLinesReached{
		if space > len(line) {
			space = len(line)
		}
		tmp := ml.buf
		if addSeparator {
			tmp = append(tmp, ml.separator...)
		}
		ml.buf = append(tmp, line[:space]...)
		ml.numLines++
	}
	ml.last = msg
}

func (ml *Multiline) Bytes() []byte{
	return ml.buf
}


func (ml *Multiline) StartTime() time.Time{
	return ml.startTime
}
