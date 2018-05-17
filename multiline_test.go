package main

import (
	"testing"
	"strings"
	"bytes"

	"github.com/docker/docker/api/types/plugins/logdriver"
)


var defaultEntry = &logdriver.LogEntry{
	Source:   "",
	TimeNano: 123456,
	Line:     nil,
	Partial:  false,
}

func TestJaveStackTrace(t *testing.T){
	content := `Exception in thread "main" java.lang.NullPointerException
	at com.example.myproject.Book.getTitle(Book.java:16)
	at com.example.myproject.Author.getBookTitles(Author.java:25)
	at com.example.myproject.Bootstrap.main(Bootstrap.java:14)
	`
	ml := Multiline{
		match:		"after",
		separator:	"\n",
		pattern: 	`^[[:space:]]`,
		negate: 	false,
		flushPtr: 	"",
		maxLines:	defaultMaxLines,
		maxBytes:   defaultMaxBytes,
		numLines:	0,
		debug:		true,
	}
	lines := strings.Split(content, "\n")
	entry := *defaultEntry
	for _, line := range lines{
		entry.Line = []byte(line)
		retVal := ml.Add(entry)
		if retVal != nil{
			t.Fatalf("Unexpected return value from multiline %s\n", string(retVal))
		}
	}

	if !bytes.Equal(ml.Bytes(), []byte(content)){
		t.Fatalf("Failed multiline - %s vs %s\n", string(ml.Bytes()), content)
	}
}

func TestTimestamp(t *testing.T){
	content := []string{`[2015-08-24 11:49:14,389]`,`[INFO ]`,`[env                      ]`,
		`[Letha] using [1] data paths`, `mounts [[/(/dev/disk1)]]`, `net usable_space [34.5gb]`,
		`net total_space [118.9gb]`, `types [hfs]`}
	ml := Multiline{
		match:		"after",
		separator:	"",
		pattern: 	`^\[[0-9]{4}-[0-9]{2}-[0-9]{2}`,
		negate: 	true,
		flushPtr: 	"",
		maxLines:	defaultMaxLines,
		maxBytes:	defaultMaxBytes,
		numLines:	0,
		debug:		true,
	}
	entry := *defaultEntry
	for _, line := range content{
		entry.Line = []byte(line)
		retVal := ml.Add(entry)
		if retVal != nil{
			t.Fatalf("Unexpected return value from multiline %s\n", string(retVal))
		}
	}
	contentStr := ""
	for _, str := range content{
		contentStr += str
	}

	if !bytes.Equal(ml.Bytes(), []byte(contentStr)){
		t.Fatalf("Failed multiline - %s vs %s\n", string(ml.Bytes()), contentStr)
	}
}

func TestLineContinuations(t *testing.T){
	content := []string{`%10.10ld  \t %10.10ld \t %s\`, `%f`}
	ml := Multiline{
		match:		"before",
		separator:	"",
		pattern: 	`\\$`,
		negate: 	false,
		flushPtr: 	"",
		maxLines:	defaultMaxLines,
		maxBytes:	defaultMaxBytes,
		numLines:	0,
		debug:		true,
	}
	entry := *defaultEntry
	for _, line := range content{
		entry.Line = []byte(line)
		retVal := ml.Add(entry)
		if retVal != nil{
			t.Fatalf("Unexpected return value from multiline %s\n", string(retVal))
		}
	}
	contentStr := ""
	for _, str := range content{
		contentStr += str
	}

	if !bytes.Equal(ml.Bytes(), []byte(contentStr)){
		t.Fatalf("Failed multiline - %s vs %s\n", string(ml.Bytes()), contentStr)
	}
}

func TestApplicationEvents(t *testing.T){
	content := []string{`Start new event`, `Logz.io Rocks!!!`, `End event`}
	ml := Multiline{
		match:		"after",
		separator:	"",
		pattern: 	`Start new event`,
		negate: 	true,
		flushPtr: 	"End event",
		maxLines:	defaultMaxLines,
		maxBytes:	defaultMaxBytes,
		numLines:	0,
		debug:		true,
	}
	entry := *defaultEntry
	for _, line := range content{
		entry.Line = []byte(line)
		retVal := ml.Add(entry)
		if retVal != nil{
			t.Fatalf("Unexpected return value from multiline %s\n", string(retVal))
		}
	}
	contentStr := ""
	for _, str := range content{
		contentStr += str
	}

	if !bytes.Equal(ml.Bytes(), []byte(contentStr)){
		t.Fatalf("Failed multiline - %s vs %s\n", string(ml.Bytes()), contentStr)
	}
}

func TestTestMultilineAfter(t *testing.T){
	content := []string{"line1", "\tline1.1", "\tline1.2", "line2", "\tline2.1", "\tline2.2"}
	ml := Multiline{
		match:		"after",
		separator:	"\n",
		pattern: 	`^[ \t] +`,
		negate: 	false,
		flushPtr: 	"",
		maxLines:	defaultMaxLines,
		maxBytes:	defaultMaxBytes,
		numLines:	0,
		debug:		true,
	}
	contentStr := ""
	for _, str := range content{
		contentStr += str
	}
	var retVal []byte
	entry := *defaultEntry
	for _, line := range content{
		entry.Line = []byte(line)
		tmpVal := ml.Add(entry)
		if tmpVal != nil{
			retVal = append(retVal, tmpVal...)
		}else if tmpVal != nil{
			t.Fatalf("Unexpected return value from multiline %s\n", string(tmpVal))
		}
	}
	retVal = append(retVal, ml.Bytes()...)
	if !bytes.Equal(retVal, []byte(contentStr)){
		t.Fatalf("Failed multiline - %s vs %s\n", string(retVal), contentStr)
	}
}

////todo - ask josh
//func TestTestMultilineBefore(t *testing.T){
//	content := []string{"line1","\\line1.1 ","\\line1.2\n", "line2","\\line2.1","\\line2.2\n"}
//	ml := Multiline{
//		match:		"before",
//		separator:	"",
//		pattern: 	"\\$",
//		negate: 	false,
//		flushPtr: 	"",
//		maxLines:	defaultMaxLines,
//		maxBytes:	defaultMaxBytes,
//		numLines:	0,
//		debug:		true,
//	}
//	contentStr := ""
//	for _, str := range content{
//		contentStr += str
//	}
//	var retVal []byte
//	for _, line := range content{
//		tmpVal := ml.Add([]byte(line))
//		if tmpVal != nil{
//			retVal = append(retVal, tmpVal...)
//		}else if tmpVal != nil{
//			t.Fatalf("Unexpected return value from multiline %s\n", string(tmpVal))
//		}
//	}
//	retVal = append(retVal, ml.Bytes()...)
//	fmt.Printf("%s\n", retVal) //todo delete
//	if !bytes.Equal(retVal, []byte(contentStr)){
//		t.Fatalf("Failed multiline - %s vs %s\n", string(retVal), contentStr)
//	}
//}

func TestMultilineAfterNegate(t *testing.T){
	content := []string{"-line1", " - line1.1", " - line1.2\n", "-line2",  " - line2.1", " - line2.2"}
	ml := Multiline{
		match:		"after",
		separator:	"",
		pattern: 	`^-`,
		negate: 	true,
		flushPtr: 	"",
		maxLines:	defaultMaxLines,
		maxBytes:	defaultMaxBytes,
		numLines:	0,
		debug:		true,
	}
	contentStr := ""
	for _, str := range content{
		contentStr += str
	}
	var retVal []byte
	entry := *defaultEntry
	for _, line := range content{
		entry.Line = []byte(line)
		tmpVal := ml.Add(entry)
		if tmpVal != nil{
			retVal = append(retVal, tmpVal...)
		}else if tmpVal != nil{
			t.Fatalf("Unexpected return value from multiline %s\n", string(tmpVal))
		}
	}
	retVal = append(retVal, ml.Bytes()...)
	if !bytes.Equal(retVal, []byte(contentStr)){
		t.Fatalf("Failed multiline - %s vs %s\n", string(retVal), contentStr)
	}
}

func TestMultilineBeforeNegate(t *testing.T){
	content := []string{"line1", "line1.1", "line1.2;\n", "line2",  "line2.1", "line2.2;"}
	ml := Multiline{
		match:		"before",
		separator:	"",
		pattern: 	`;$`,
		negate: 	true,
		flushPtr: 	"",
		maxLines:	defaultMaxLines,
		maxBytes:	defaultMaxBytes,
		numLines:	0,
		debug:		true,
	}
	contentStr := ""
	for _, str := range content{
		contentStr += str
	}
	var retVal []byte
	entry := *defaultEntry
	for _, line := range content{
		entry.Line = []byte(line)
		tmpVal := ml.Add(entry)
		if tmpVal != nil{
			retVal = append(retVal, tmpVal...)
		}else if tmpVal != nil{
			t.Fatalf("Unexpected return value from multiline %s\n", string(tmpVal))
		}
	}
	retVal = append(retVal, ml.Bytes()...)
	if !bytes.Equal(retVal, []byte(contentStr)){
		t.Fatalf("Failed multiline - %s vs %s\n", string(retVal), contentStr)
	}
}

func TestMultilineAfterNegateFlushPattern(t *testing.T){
	content := []string{"EventStart", "EventId: 1", "EventEnd\n", "OtherThingInBetween\n", "EventStart", "EventId: 2", "EventEnd"}
	ml := Multiline{
		match:		"after",
		separator:	"",
		pattern: 	`EventStart`,
		negate: 	true,
		flushPtr: 	"EventEnd",
		maxLines:	defaultMaxLines,
		maxBytes:	defaultMaxBytes,
		numLines:	0,
		debug:		true,
	}
	contentStr := ""
	for _, str := range content{
		contentStr += str
	}
	var retVal []byte
	entry := *defaultEntry
	for _, line := range content{
		entry.Line = []byte(line)
		tmpVal := ml.Add(entry)
		if tmpVal != nil{
			retVal = append(retVal, tmpVal...)
		}else if tmpVal != nil{
			t.Fatalf("Unexpected return value from multiline %s\n", string(tmpVal))
		}
	}
	retVal = append(retVal, ml.Bytes()...)
	numOfEvents := len(strings.Split(string(retVal), "\n"))
	if numOfEvents != 3{
		t.Fatalf("Unexpected number of event - %d vs 3\n", numOfEvents)
	}

	if !bytes.Equal(retVal, []byte(contentStr)){
		t.Fatalf("Failed multiline - %s vs %s\n", string(retVal), contentStr)
	}
}

func TestMultilineAfterNegateFlushPatternWhereTheFirstLinesDosentMatchTheStartPattern(t *testing.T){
	content := []string{"StartLineThatDosentMatchTheEvent\n", "OtherThingInBetween\n", "EventStart", "EventId: 2",
	"EventEnd\n", "EventStart","EventId: 3", "EventEnd",}
	ml := Multiline{
		match:		"after",
		separator:	"",
		pattern: 	`EventStart`,
		negate: 	true,
		flushPtr: 	"EventEnd",
		maxLines:	defaultMaxLines,
		maxBytes:	defaultMaxBytes,
		numLines:	0,
		debug:		true,
	}
	contentStr := ""
	for _, str := range content{
		contentStr += str
	}
	var retVal []byte
	entry := *defaultEntry
	for _, line := range content{
		entry.Line = []byte(line)
		tmpVal := ml.Add(entry)
		if tmpVal != nil{
			retVal = append(retVal, tmpVal...)
		}else if tmpVal != nil{
			t.Fatalf("Unexpected return value from multiline %s\n", string(tmpVal))
		}
	}
	retVal = append(retVal, ml.Bytes()...)
	numOfEvents := len(strings.Split(string(retVal), "\n"))
	if numOfEvents != 4{
		t.Fatalf("Unexpected number of event - %d vs 4\n", numOfEvents)
	}

	if !bytes.Equal(retVal, []byte(contentStr)){
		t.Fatalf("Failed multiline - %s vs %s\n", string(retVal), contentStr)
	}
}


func TestMultilineBeforeNegateOKWithEmptyLine(t *testing.T){
	content := []string{"StartLineThatDosentMatchTheEvent\n", "OtherThingInBetween\n", "EventStart", "EventId: 2",
		"EventEnd\n", "EventStart","EventId: 3", "EventEnd",}
	ml := Multiline{
		match:		"after",
		separator:	"",
		pattern: 	`EventStart`,
		negate: 	true,
		flushPtr: 	"EventEnd",
		maxLines:	defaultMaxLines,
		maxBytes:	defaultMaxBytes,
		numLines:	0,
		debug:		true,
	}
	contentStr := ""
	for _, str := range content{
		contentStr += str
	}
	var retVal []byte
	entry := *defaultEntry
	for _, line := range content{
		entry.Line = []byte(line)
		tmpVal := ml.Add(entry)
		if tmpVal != nil{
			retVal = append(retVal, tmpVal...)
		}else if tmpVal != nil{
			t.Fatalf("Unexpected return value from multiline %s\n", string(tmpVal))
		}
	}
	retVal = append(retVal, ml.Bytes()...)
	numOfEvents := len(strings.Split(string(retVal), "\n"))
	if numOfEvents != 4{
		t.Fatalf("Unexpected number of event - %d vs 4\n", numOfEvents)
	}

	if !bytes.Equal(retVal, []byte(contentStr)){
		t.Fatalf("Failed multiline - %s vs %s\n", string(retVal), contentStr)
	}
}