package elog

import (
	"fmt"
	"os"
	"path"
	"sync"
	"time"
)

const (
	TimeFormat = "2006/01/02 15:04:05"
)

/**
 * 日志相关处理
 */
type EbhLog struct {
	LogPath string         //日志存放路径
	Out     *Hander        //日志输出对象
	Mu      sync.Mutex     //日志锁
	quit    chan struct{}  //退出锁
	W       sync.WaitGroup //等待组，用于阻塞日志运行
	Msg     chan []byte    //信息信道
	Level   LogLevel
}
type LogLevel int

const (
	SYS LogLevel = iota
	FATAL
	ERROR
	WARN
	INFO
	TRACE
)

var LevelName []string = []string{"sys", "fatal", "error", "warn", "info", "debug"}

type Hander struct {
	out *os.File
}

func (h *Hander) Write(p []byte) (n int, err error) {
	return h.out.Write(p)
}
func (h *Hander) Close() {
	h.out.Close()
}
func NewStdHander() *Hander {
	stdHander := new(Hander)
	stdHander.out = os.Stdout
	return stdHander
}
func NewFileHander(filename string) (*Hander, error) {
	dir := path.Dir(filename)
	os.Mkdir(dir, 0777)
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return nil, err
	}
	h := new(Hander)
	h.out = f
	return h, nil
}

func New(h *Hander) *EbhLog {
	l := new(EbhLog)
	l.quit = make(chan struct{})
	l.Msg = make(chan []byte, 1024)
	l.Out = h
	l.Level = INFO
	l.W.Add(1)
	go l.runLog()
	return l
}

func (l *EbhLog) runLog() {
	defer l.W.Done()
	for {
		select {
		case msg := <-l.Msg:
			l.Out.Write(msg)
		case <-l.quit:
			return
		}
	}
}

/**
 * 获取默认日志器
 */
func GetStdLogger() *EbhLog {
	std := NewStdHander()
	return New(std)
}

var ELog *EbhLog = GetStdLogger()

func (l *EbhLog) log(level LogLevel, msg string) {
	if level > l.Level {
		return
	}
	if ELog == nil {
		fmt.Println("log init error")
		return
	}
	if level < 0 {
		level = INFO
	}
	var buf []byte = make([]byte, 0, 1024)
	buf = append(buf, "["+LevelName[level]+"]"...)
	curtime := time.Now().Format(TimeFormat)
	buf = append(buf, curtime...)
	buf = append(buf, " "...)
	buf = append(buf, msg...)
	if buf[len(buf)-1] != '\n' {
		buf = append(buf, '\n')
	}
	l.Msg <- buf
}
func (l *EbhLog) SetLevel(level LogLevel) {
	l.Level = level
}
func Trace(v ...interface{}) {
	ELog.log(TRACE, fmt.Sprint(v...))
}
func Info(v ...interface{}) {
	ELog.log(INFO, fmt.Sprint(v...))
}
func Warn(v ...interface{}) {
	ELog.log(WARN, fmt.Sprint(v...))
}
func Error(v ...interface{}) {
	ELog.log(ERROR, fmt.Sprint(v...))
}
func Fatal(v ...interface{}) {
	ELog.log(FATAL, fmt.Sprint(v...))
	ELog.close()
	os.Exit(1)
}
func Sys(v ...interface{}) {
	ELog.log(SYS, fmt.Sprint(v...))
}

func (l *EbhLog) close() {
	close(l.quit)
	l.W.Wait()
	l.quit = nil
	l.Out.Close()
}
func Close() {
	ELog.close()
}
