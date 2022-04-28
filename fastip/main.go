package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/Dreamacro/clash/adapter/outbound"
	"github.com/Dreamacro/clash/constant"
	"github.com/darabuchi/log"
	"github.com/darabuchi/utils"
	"github.com/ncruces/go-dns"
	"github.com/pterm/pterm"
)

var p *pterm.ProgressbarPrinter

func main() {
	log.SetLevel(log.FatalLevel)

	p, _ = pterm.DefaultProgressbar.
		WithTitle("check host").
		WithShowElapsedTime(true).
		WithShowCount(true).
		WithShowTitle(true).
		WithShowPercentage(true).
		WithElapsedTimeRoundingFactor(time.Second).
		WithTotal(len(hostList)*len(nameserverList) + 1).Start()

	hostList.Each(func(host string) {
		ip, err := fast(host)
		if err != nil {
			pterm.Error.Printfln("err:%v", err)
			return
		}
		// fmt.Printf("%s\t%s\n", host, ip)
		// log.Infof("%s\t%s\n", host, ip)
		pterm.Success.Printfln("%s\t%s", host, ip)
	})
	_, _ = p.Stop()
}

func fast(host string) (net.IP, error) {
	log.Infof("handel %s", host)

	ipMap := map[string]net.IPAddr{}
	nameserverList.Each(func(nameserver string) {
		ips, err := query(nameserver, host)
		if err != nil {
			log.Errorf("err:%v", err)
			return
		}

		for _, ip := range ips {
			ipMap[ip.String()] = ip
		}
	})

	// p.Total += len(ipMap)

	direct := outbound.NewDirect()

	fastIp := net.IPAddr{}
	fastDelay := time.Duration(-1)
	for _, ip := range ipMap {
		p.UpdateTitle(fmt.Sprintf("check fastest for %s", host))
		p.Increment()

		log.Infof("check %s for %s", ip.String(), host)

		client := http.Client{
			Transport: &http.Transport{
				Proxy: nil,
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					return direct.DialContext(ctx, &constant.Metadata{
						NetWork:  constant.TCP,
						DstIP:    ip.IP,
						DstPort:  "443",
						AddrType: constant.AtypDomainName,
						Host:     host,
						DNSMode:  constant.DNSFakeIP,
					})
				},
				DialTLSContext:        nil,
				TLSHandshakeTimeout:   time.Second * 3,
				DisableKeepAlives:     true,
				DisableCompression:    true,
				MaxIdleConns:          1,
				MaxIdleConnsPerHost:   1,
				MaxConnsPerHost:       1,
				IdleConnTimeout:       time.Second * 3,
				ResponseHeaderTimeout: time.Second * 3,
				ExpectContinueTimeout: time.Minute,
			},
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
			Timeout: time.Second * 5,
		}

		start := time.Now()
		_, _ = client.Get(host)
		delay := time.Since(start)

		log.Infof("%s use %s delay %v", host, ip.String(), delay)

		if fastDelay < 0 || fastDelay > delay {
			fastDelay = delay
			fastIp = ip
		}
	}

	if fastDelay < 0 {
		return nil, errors.New("fastip: no ip")
	}

	return fastIp.IP, nil
}

func query(nameserver, hostname string) ([]net.IPAddr, error) {
	p.UpdateTitle(fmt.Sprintf("query ip for %s", hostname))
	p.Increment()

	log.Infof("query ip for %s from %s", hostname, nameserver)

	var client *net.Resolver
	var err error
	if utils.IsIp(nameserver) {
		client, err = dns.NewDoTResolver(nameserver)
		if err != nil {
			log.Debugf("err:%v", err)
			return nil, err
		}
	} else {
		u, err := url.Parse(nameserver)
		if err != nil {
			log.Errorf("err:%v", err)
			u = &url.URL{}
		}

		switch u.Scheme {
		case "":
			client, err = dns.NewDoTResolver(nameserver)
			if err != nil {
				log.Debugf("err:%v", err)
				return nil, err
			}
		case "tls", "http", "https":
			fallthrough
		default:
			client, err = dns.NewDoHResolver(nameserver)
			if err != nil {
				log.Debugf("err:%v", err)
				return nil, err
			}
		}
	}

	ips, err := client.LookupIPAddr(context.TODO(), hostname)
	if err != nil {
		log.Errorf("err:%v", err)
		return nil, err
	}

	for _, ip := range ips {
		log.Infof("found %s from %s for %s", ip.String(), nameserver, hostname)
	}

	return ips, nil
}
