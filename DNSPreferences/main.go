package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/Luoxin/Eutamias/utils"
	"github.com/alexflint/go-arg"
	"github.com/apex/log"
	"github.com/cloverstd/tcping/ping"
	"github.com/ncruces/go-dns"
	"math/rand"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const timeout = time.Millisecond * 500

type NameserverCheck struct {
	NameServer string

	PingTotalDuration time.Duration
	PingFailed        uint64
	PingSuccess       uint64
	PingCounter       uint64

	DnsPingTotalDuration time.Duration
	DnsPingFailed        uint64
	DnsPingSuccess       uint64
	DnsPingCounter       uint64
}

func NewNameserverCheck(nameserver string) *NameserverCheck {
	return &NameserverCheck{
		NameServer: nameserver,
	}
}

func (p *NameserverCheck) Check() {
	if p.PingSelf() != nil {
		return
	}

	if p.Query() != nil {
		return
	}
}

func (p *NameserverCheck) Query() error {
	var client *net.Resolver
	var err error
	if utils.IsIp(p.NameServer) {
		client, err = dns.NewDoTResolver(p.NameServer)
		if err != nil {
			log.Debugf("err:%v", err)
			return err
		}
	} else {
		u, err := url.Parse(p.NameServer)
		if err != nil {
			log.Errorf("err:%v", err)
			return err
		}

		switch u.Scheme {
		case "":
			client, err = dns.NewDoTResolver(p.NameServer)
			if err != nil {
				log.Debugf("err:%v", err)
				return err
			}
		case "tls", "http", "https":
			fallthrough
		default:
			client, err = dns.NewDoHResolver(p.NameServer)
			if err != nil {
				log.Debugf("err:%v", err)
				return err
			}
		}
	}

	ips, err := client.LookupIP(context.TODO(), "ip4", "baidu.com")
	if err != nil {
		log.Errorf("err:%v", err)
		return err
	}

	if len(ips) == 0 {
		return errors.New("not found ip")
	}

	target := ping.Target{
		Timeout:  timeout,
		Interval: time.Nanosecond,
		Host:     "baidu.com",
		Counter:  10,
		Port:     443,
		Protocol: ping.HTTPS,
	}

	pinger := ping.NewTCPing()
	pinger.SetTarget(&target)
	pingerDone := pinger.Start()
	<-pingerDone
	if pinger.Result().Failed() == pinger.Result().Counter {
		return errors.New("ping not working")
	}

	p.DnsPingFailed += uint64(pinger.Result().Failed())
	p.DnsPingSuccess += uint64(pinger.Result().SuccessCounter)
	p.DnsPingCounter += uint64(pinger.Result().Counter)
	p.DnsPingTotalDuration += pinger.Result().TotalDuration

	return nil
}

func (p *NameserverCheck) PingSelf() error {
	var host string
	port := 53
	protocol := ping.TCP

	if utils.IsIpV4(p.NameServer) {
		host = p.NameServer
	} else if utils.IsIpV6(p.NameServer) {
		host = fmt.Sprintf("[%s]", strings.TrimPrefix(strings.TrimPrefix(p.NameServer, "["), "]"))
	} else {
		u, err := url.Parse(p.NameServer)
		if err != nil {
			log.Errorf("err:%v", err)
			return err
		}

		switch u.Scheme {
		case "", "tls":
			if u.Port() != "" {
				p, err := strconv.ParseInt(u.Port(), 10, 32)
				if err != nil {
					log.Errorf("err:%v", err)
					return err
				}
				port = int(p)
			}

		case "http":
			if u.Port() != "" {
				p, err := strconv.ParseInt(u.Port(), 10, 32)
				if err != nil {
					log.Errorf("err:%v", err)
					return err
				}
				port = int(p)
			} else {
				port = 80
			}

			protocol = ping.HTTP

		case "https":
			if u.Port() != "" {
				p, err := strconv.ParseInt(u.Port(), 10, 32)
				if err != nil {
					log.Errorf("err:%v", err)
					return err
				}
				port = int(p)
			} else {
				port = 443
			}
			protocol = ping.HTTPS
		}

		host = u.Hostname()
	}

	target := ping.Target{
		Timeout:  timeout,
		Interval: time.Nanosecond,
		Host:     host,
		Counter:  10,
		Port:     port,
		Protocol: protocol,
		Proxy:    cmdArgs.Proxy,
	}

	pinger := ping.NewTCPing()
	pinger.SetTarget(&target)
	pingerDone := pinger.Start()
	<-pingerDone
	if pinger.Result().Failed() == pinger.Result().Counter {
		return errors.New("ping not working")
	}

	p.PingFailed += uint64(pinger.Result().Failed())
	p.PingSuccess += uint64(pinger.Result().SuccessCounter)
	p.PingCounter += uint64(pinger.Result().Counter)
	p.PingTotalDuration += pinger.Result().TotalDuration

	return nil
}

var cmdArgs struct {
	Proxy string `arg:"-p,--proxy" help:"check with proxy"`
}

func main() {
	arg.MustParse(&cmdArgs)

	var nameserverCheckList NameserverCheckList

	nameserverList.Each(func(nameserver string) {
		check := NewNameserverCheck(nameserver)
		nameserverCheckList = append(nameserverCheckList, check)
	})

	nameserverCheckList.
		Shuffle(rand.NewSource(time.Now().UnixNano())).
		Each(func(check *NameserverCheck) {
			check.Check()
		}).
		Shuffle(rand.NewSource(time.Now().UnixNano())).
		Each(func(check *NameserverCheck) {
			check.Check()
		}).
		Shuffle(rand.NewSource(time.Now().UnixNano())).
		Each(func(check *NameserverCheck) {
			check.Check()
		}).
		Shuffle(rand.NewSource(time.Now().UnixNano())).
		Each(func(check *NameserverCheck) {
			check.Check()
		}).
		FilterNot(func(check *NameserverCheck) bool {
			return check.PingCounter == 0 || check.DnsPingCounter == 0
		}).
		Filter(func(check *NameserverCheck) bool {
			return check.PingFailed == 0 && check.DnsPingFailed == 0
		}).
		SortUsing(func(a, b *NameserverCheck) bool {
			return a.PingTotalDuration/time.Duration(a.PingSuccess) < b.PingTotalDuration/time.Duration(b.PingSuccess)
		}).
		Each(func(check *NameserverCheck) {
			//fmt.Println(fmt.Sprintf("%v——%v", check.NameServer, check.PingTotalDuration/time.Duration(check.PingSuccess)))
			fmt.Println(check.NameServer)
		})
}
