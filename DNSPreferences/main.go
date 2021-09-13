package main

import (
	"errors"
	"fmt"
	"github.com/Luoxin/Eutamias/utils"
	"github.com/apex/log"
	"github.com/cloverstd/tcping/ping"
	"net"
	"net/url"
	"strconv"
	"time"
)

const timeout = time.Millisecond * 500

type NameserverCheck struct {
	NameServer string

	IsFail   bool
	PingAvg  time.Duration
	PingPate float64
}

func NewNameserverCheck(nameserver string) *NameserverCheck {
	return &NameserverCheck{
		NameServer: nameserver,
	}
}

func (p *NameserverCheck) Check() {
	if p.PingSelf() != nil {
		p.IsFail = true
		return
	}
}

func (p *NameserverCheck) PingSelf() error {
	var host string
	port := 53
	protocol := ping.TCP

	if utils.IsIp(p.NameServer) {
		host = p.NameServer
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

		//switch u.Scheme {
		//case "tls":
		//	if strings.Contains(u.Path, "@") {
		//		u, err = url.Parse(strings.Replace(p.NameServer, "@", ":", 1))
		//		if err != nil {
		//			log.Errorf("err:%v", err)
		//			return err
		//		}
		//	}
		//}

		ip, err := net.LookupIP(u.Hostname())
		if err != nil {
			log.Errorf("err:%v", err)
			return err
		}

		if len(ip) == 0 {
			return errors.New("not found ips")
		}

		host = ip[0].String()
	}

	target := ping.Target{
		Timeout:  timeout,
		Interval: time.Nanosecond,
		Host:     host,
		Counter:  10,
		Port:     port,
		Protocol: protocol,
	}

	pinger := ping.NewTCPing()
	pinger.SetTarget(&target)
	pingerDone := pinger.Start()
	<-pingerDone
	if pinger.Result().Failed() == pinger.Result().Counter {
		return errors.New("ping not working")
	}

	p.PingAvg = pinger.Result().Avg()
	p.PingPate = float64(pinger.Result().Failed()) / float64(pinger.Result().Counter)

	return nil
}

func main() {
	var nameserverCheckList NameserverCheckList

	nameserverList.Each(func(nameserver string) {
		check := NewNameserverCheck(nameserver)
		check.Check()
		nameserverCheckList = append(nameserverCheckList, check)
	})

	nameserverCheckList.
		FilterNot(func(check *NameserverCheck) bool {
			return check.IsFail
		}).
		Filter(func(check *NameserverCheck) bool {
			return check.PingPate == 0
		}).
		//SortUsing(func(a, b *NameserverCheck) bool {
		//	return a.PingPate > b.PingPate
		//}).
		SortUsing(func(a, b *NameserverCheck) bool {
			return a.PingAvg < b.PingAvg
		}).
		Each(func(check *NameserverCheck) {
			//fmt.Println(fmt.Sprintf("%v——%v——%v", check.NameServer, check.PingAvg, check.PingPate))
			fmt.Println(check.NameServer)
		})
}
