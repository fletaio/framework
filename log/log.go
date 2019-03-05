package log

import (
	"fmt"
	"log"
)

const (
	// DefaultTimeFormat is the default time format when parsing Time values.
	// it is exposed to allow handlers to use and not have to redefine
	DefaultTimeFormat = "2006-01-02T15:04:05.000000000Z07:00"
)

// Debug logs a debug entry
func Debug(v ...interface{}) {
	// Msg(v...)
	v = append([]interface{}{"DEBUG "}, v...)
	log.Println(v)
}

// Debugf logs a debug entry with formatting
func Debugf(s string, v ...interface{}) {
	// Msgf(s, v...)
	s = "DEBUG " + s
	log.Printf(s, v)
}

// Info logs a normal. information, entry
func Info(v ...interface{}) {
	v = append([]interface{}{"INFO "}, v...)
	log.Println(v)
}

// Infof logs a normal. information, entry with formatiing
func Infof(s string, v ...interface{}) {
	s = "INFO " + s
	log.Printf(s, v)
}

// Panic logs a panic log entry
func Panic(v ...interface{}) {
	v = append([]interface{}{"Panic "}, v...)
	panic(fmt.Sprintln(v))
}

// Panicf logs a panic log entry with formatting
func Panicf(s string, v ...interface{}) {
	s = "NOTICE " + s
	panic(fmt.Sprintf(s, v))
}

// Notice logs a notice log entry
func Notice(v ...interface{}) {
	v = append([]interface{}{"NOTICE "}, v...)
	log.Println(v)
}

// Noticef logs a notice log entry with formatting
func Noticef(s string, v ...interface{}) {
	s = "NOTICE " + s
	log.Printf(s, v)
}

// Warn logs a warn log entry
func Warn(v ...interface{}) {
	v = append([]interface{}{"WARN "}, v...)
	log.Println(v)
}

// Warnf logs a warn log entry with formatting
func Warnf(s string, v ...interface{}) {
	s = "WARN " + s
	log.Printf(s, v)
}

// Error logs an error log entry
func Error(v ...interface{}) {
	v = append([]interface{}{"ERROR "}, v...)
	// Msg(v...)
	log.Println(v)
}

// Errorf logs an error log entry with formatting
func Errorf(s string, v ...interface{}) {
	s = "ERROR " + s
	// Msgf(s, v...)
	log.Printf(s, v)
}
