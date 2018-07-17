package main

import (
	"github.com/lchh5/httpchecker/common/elog"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

var check Checker
var isdone chan bool
var path string

const (
	TimeFormat = "2006/01/02 15:04:05"
)

type Job struct {
	Descript    string `json:"descript"`
	Method      string `json:"method"`
	Url         string `json:"url"`
	Host        string `json:"host"`
	ContentType string `json:"contentType"`
	Data        string `json:"data"`
	Duration    int    `json:"duration"`
}
type Checker struct {
	Logpath string `json:"log"`
	Webhook string `json:"webhook"`
	Joblist []Job  `json:"joblist"`
}

func main() {
	InitParam()
	loadresult := LoadChecker(path, &check)
	if !loadresult {
		fmt.Println("load config fail")
		return
	}
	PrintChecker(check)
	deamon()
	isdone = make(chan bool)
	//捕获异常
	defer handlePanic()
	logpath := check.Logpath
	filehander, err := elog.NewFileHander(logpath)
	if err == nil {
		elog.ELog = elog.New(filehander)
	}
	joblist := check.Joblist
	if joblist == nil {
		fmt.Println("job list is nil")
	} else {
		for i := 0; i < len(joblist); i++ {
			job := joblist[i]
			go RunJob(job)
		}
	}
	if <-isdone {
		fmt.Println("is done")
	}
}
func RunJob(job Job) {
	haserror := false //用来标识当前是否有错误了
	errcount := 0     //出错次数，提示两次错误 除非恢复了 否则不再提示了
	if job.Url == "" {
		return
	}
	method := "GET"
	if job.Method == "post" {
		method = "POST"
	}
	payload := strings.NewReader(job.Data)
	for {
		req, _ := http.NewRequest(method, job.Url, payload)
		if job.ContentType != "" {
			req.Header.Add("Content-Type", job.ContentType)
		}
		// req.Header.Add("User-Agent", "ebh http checker 1.0") Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/63.0.3239.132 Safari/537.36
		req.Header.Add("User-Agent", "Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/63.0.3239.132 Safari/537.36 ebh http checker 1.0")
		if job.Host != "" {
			req.Host = job.Host
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil || res.StatusCode != 200 {

			haserror = true
			errcount++
			if errcount < 3 { //同样消息只提醒两次
				msg := "无"
				if err != nil {
					msg = err.Error()
				}
				curtime := time.Now().Format(TimeFormat)
				statuscode := -1
				if res != nil {
					statuscode = res.StatusCode
				}
				errmsg := fmt.Sprintf("%s 访问异常 \r\n URL:%s\r\n StatusCode:%d\r\n错误信息:%s\r\n时间：%s\r\n", job.Descript, job.Url, statuscode, msg, curtime)
				NotifyError(errmsg)
			}
		} else if haserror {
			haserror = false
			errcount = 0
			curtime := time.Now().Format(TimeFormat)
			errmsg := fmt.Sprintf("%s 恢复正常 \r\nURL:%s\r\nStatusCode:%d\r\n时间:%s\r\n", job.Descript, job.Url, res.StatusCode, curtime)
			NotifyError(errmsg)
		}
		time.Sleep(time.Second * time.Duration(job.Duration))
	}
}

type DingResult struct {
	Errmsg  string `json:"errmsg"`
	Errcode int    `json:"errcode"`
}

func NotifyError(msg string) {
	elog.Error(msg)
	url := check.Webhook
	notifymsg := fmt.Sprintf("{\"msgtype\":\"text\",\"text\":{\"content\": \"%s\"}}", msg)
	payload := strings.NewReader(notifymsg)
	req, rerr := http.NewRequest("POST", url, payload)
	if rerr != nil {
		elog.Error(rerr.Error())
		return
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Cache-Control", "no-cache")

	res, derr := http.DefaultClient.Do(req)
	if derr != nil {
		elog.Error("发送钉钉提醒消息时出错 " + notifymsg + rerr.Error())
		return
	}
	defer res.Body.Close()
	body, dingerr := ioutil.ReadAll(res.Body)
	if dingerr != nil {
		elog.Error(rerr.Error())
		return
	}
	var dingresult DingResult
	jerr := json.Unmarshal(body, &dingresult)
	if jerr != nil {
		elog.Error(jerr.Error())
		return
	}
	if dingresult.Errcode != 0 {
		elog.Error(fmt.Sprintf("发送钉钉消息时候出错,错误码：%d 错误消息: %s", dingresult.Errcode, dingresult.Errmsg))
	}
}
func LoadChecker(config string, check interface{}) bool {
	data, err := ioutil.ReadFile(config)
	if err != nil {
		fmt.Println(err.Error())
		return false
	}
	err = json.Unmarshal(data, check)
	if err != nil {
		fmt.Println(err.Error())
		return false
	}
	return true
}
func PrintChecker(check Checker) {
	fmt.Println("Logpath:", check.Logpath)
	fmt.Println("Webhook:", check.Webhook)
	joblist := check.Joblist
	if joblist == nil {
		fmt.Println("job list is nil")
	} else {
		for i := 0; i < len(joblist); i++ {
			job := joblist[i]
			fmt.Printf("job %d --------------\n", i)
			fmt.Printf("url:%s\nhost:%s\nmethod:%s\ncontentType:%s\ndata:%s\nduration:%d\n\n", job.Url, job.Host, job.Method, job.ContentType, job.Data, job.Duration)
		}
	}
}

/**
 * 守护进程
 * @return {[type]} [description]
 */
func deamon() {
	//创建监听退出chan
	c := make(chan os.Signal)
	signal.Notify(c, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go notifySignal(c)
}

/**
 * 处理进程信号
 * @return {[type]} [description]
 */
func notifySignal(ch chan os.Signal) {
	for s := range ch {
		switch s {
		case syscall.SIGHUP:
			elog.Sys("收到 SIGHUP 信号，略过", s)
		case syscall.SIGINT:
			elog.Sys("收到 SIGINT 退出信号，系统退出", s)
			quit()
		case syscall.SIGTERM:
			elog.Sys("收到 SIGTERM 退出信号，系统退出", s)
			quit()
		case syscall.SIGQUIT:
			elog.Sys("收到 SIGQUIT 信号，略过", s)

		default:
			elog.Sys("收到其他信号", s)
		}
	}
}
func quit() {
	elog.Sys("成功退出")
	elog.Close()
	close(isdone)
	os.Exit(0)
}

/**
 * 捕获异常
 * @return {[type]} [description]
 */
func handlePanic() {
	if err := recover(); err != nil {
		fmt.Println(err)
		elog.Error(err)
	}
}
func InitParam() bool {
	flag.StringVar(&path, "c", "", "config path ")
	flag.Parse()
	if path == "" {
		path = "config.json"
	}
	return true
}
