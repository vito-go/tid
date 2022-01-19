// Package tid 生成traceId进行日志追踪，基于时间纳秒级别。本机全局唯一.
package tid

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type tide struct {
	mux sync.Mutex
	t   int64
}

func newTid() *tide {
	return &tide{mux: sync.Mutex{}}
}

var tid = newTid()

// rpcRetryLimit 最多尝试100次（约为10分钟）
const rpcRetryLimit = 100

// _ipCode 全局本机ip所对应的code.
var _ipCode int64

func Get() int64 {
	return tid.get()
}

// Register can only do once.
// unilogAddr 分布式日志统一中心地址.
func Register(appName string, unilogAddr string) error {
	if unilogAddr == "" {
		return errors.New("unilogAddr empty")
	}
	if _ipCode == 0 {
		return errors.New("_ipCode is zero")
	}
	go run(appName, unilogAddr)
	return nil
}

func init() {
	priIP, err := getPrivateIP()
	if err != nil {
		log.Println(err)
	}
	var ipCodeStr string
	if ss := strings.Split(priIP, "."); len(ss) == 4 {
		ipCodeStr = ss[3]
	}
	_ipCode, err = strconv.ParseInt(ipCodeStr, 10, 64)
	if err != nil {
		log.Printf("ipcode init error. priIP=%s ipCodeStr=%s err=%s\n", priIP, ipCodeStr, err.Error())
	}
}

// start. 第一个for循环.
func run(appName, unilogAddr string) {
	var conn net.Conn
	var err error
	var rpcCli *rpc.Client
	var retryCount int
	for {
		if retryCount >= rpcRetryLimit {
			// 重试超过100次就终止
			return
		}
		conn, err = net.Dial("tcp", unilogAddr)
		if err != nil {
			// mylog.Ctx(context.TODO()).Errorf("unilog服务链接错误, 10s后重试验. err: %s", err.Error())
			time.Sleep(time.Second * 6)
			retryCount++
			continue
		}
		rpcCli = rpc.NewClient(conn)
		for {
			var ip string
			if ss := strings.Split(conn.LocalAddr().String(), ":"); len(ss) == 2 {
				ip = ss[0]
			}
			// ipCode是0就代表第一次发
			var code = atomic.LoadInt64(&_ipCode)
			err = rpcCli.Call("Server.Register", fmt.Sprintf("%s__%s__%d", appName, ip, _ipCode), &code)
			if err != nil {
				// 大概率是unilog服务器中断
				break
			}
			atomic.StoreInt64(&_ipCode, code)
			retryCount = 0 // 正常的重启
			time.Sleep(time.Second * 3)
		}
	}
}

func (u *tide) get() int64 {
	u.mux.Lock()
	defer u.mux.Unlock()
	// go1.14  time.Now().UnixMicro undefined (type time.Time has no field or method UnixMicro)
	// t := time.Now().UnixMicro()
	t := time.Now().UnixNano()/1e3*1e3 + atomic.LoadInt64(&_ipCode)
	if u.t == t {
		t++
	}
	u.t = t
	return t
}
