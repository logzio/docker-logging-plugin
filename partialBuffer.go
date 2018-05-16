package main

import (
	"github.com/docker/docker/api/types/plugins/logdriver"

	"time"
)

type PartialBuffer struct {
	buf       []byte
	maxBytes  int
	startTime time.Time
	source    string
	timeNano  int64
	timeout   time.Duration
}

func (pb *PartialBuffer) Add(entry logdriver.LogEntry) {
	if len(pb.buf) == 0 {
		pb.source = entry.Source
		pb.timeNano = entry.TimeNano
	}
	line := entry.Line
	sz := len(line)
	//space left
	space := pb.maxBytes - sz

	// do we have enough space?
	if space > 0 {
		if space > len(line) {
			space = len(line)
		}
		pb.buf = append(pb.buf, line[:space]...)
	}
}

func (pb *PartialBuffer) Reset() {
	pb.buf = nil
	pb.startTime = time.Now()
	pb.timeNano = 0
	pb.source = ""
}
