package main

import (
	"github.com/docker/docker/api/types/plugins/logdriver"

	"time"
)

type PartialBuffer struct {
	buf       []byte
	maxBytes  int
	startTime time.Time
	timeout   time.Duration
	last	  logdriver.LogEntry
}

func (pb *PartialBuffer) Add(entry logdriver.LogEntry) {
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
	pb.last = entry
}

func (pb *PartialBuffer) Reset() {
	pb.buf = nil
	pb.startTime = time.Now()
}

func (pb *PartialBuffer) Flush() logdriver.LogEntry{
	return logdriver.LogEntry{
		Source:		pb.last.Source,
		TimeNano: 	pb.last.TimeNano,
		Line:		pb.buf,
		Partial:  	pb.last.Partial,
	}
}

func (pb *PartialBuffer) Timeout() time.Duration{
	return pb.timeout
}