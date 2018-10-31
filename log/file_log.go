package log

import (
	"flag"
	"log"
	"os"
	"sync"
	"time"
)

var logfile *os.File

var (
	logger *log.Logger
)

const (
	format   string = "2006-01-02T15:04:05.999999999"
	zerofill string = "0000000"
)

func init() {
	wd, _ := os.Getwd()
	var logpath = wd + "/file_log.log"

	flag.Parse()
	var file, err1 = os.Create(logpath)

	if err1 != nil {
		panic(err1)
	}
	logger = log.New(file, "", log.LstdFlags|log.Lshortfile)
	logger.SetFlags(0)

	addrMap = make(map[string]string)
}

var (
	addrMap     map[string]string
	addrMapLock sync.Mutex
)

//Msg is file write log
func Msg(msg ...interface{}) {
	msg = append([]interface{}{string([]byte(time.Now().Format(format) + zerofill)[:27]), " "}, msg...)
	logger.Print(msg...)
}

//Msgf is file write log
func Msgf(f string, msg ...interface{}) {
	logger.Printf(string([]byte(time.Now().Format(format) + zerofill)[:27])+" "+f, msg...)
}
