package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Dreamacro/clash/adapter/outbound"
	"github.com/Dreamacro/clash/constant"
	"github.com/darabuchi/log"
	"github.com/darabuchi/utils"
	"github.com/elliotchance/pie/pie"
	"github.com/ncruces/go-dns"
	"github.com/pterm/pterm"
	"github.com/txn2/txeh"
)

type IpOptimization struct {
	nameserverList pie.Strings
	domainList     pie.Strings

	lock    sync.RWMutex
	bar     *pterm.ProgressbarPrinter
	taskCnt int
	start   time.Time
	hosts   *txeh.Hosts
}

func NewIpOptimization() *IpOptimization {
	p := &IpOptimization{
		start: time.Now(),
	}

	p.bar, _ = pterm.DefaultProgressbar.
		WithTitle("check host").
		WithShowElapsedTime(true).
		WithShowCount(true).
		WithShowTitle(true).
		WithShowPercentage(true).
		WithElapsedTimeRoundingFactor(time.Second).
		WithTotal(p.taskCnt).
		Start()

	return p
}

func (p *IpOptimization) incrBy(cnt int) {
	if cnt == 0 {
		return
	}

	p.lock.Lock()
	defer p.lock.Unlock()

	p.taskCnt += cnt
	p.bar = p.bar.WithTotal(p.taskCnt)
}

func (p *IpOptimization) increment(format string, a ...interface{}) {
	p.lock.RLock()
	defer p.lock.RUnlock()
	msg := fmt.Sprintf(format, a...)
	if msg != "" {
		p.bar.UpdateTitle(msg)
	}

	p.bar.Increment()
}

func (p *IpOptimization) AddNameService(nameserverList ...string) *IpOptimization {
	p.nameserverList = append(p.nameserverList, nameserverList...)
	return p
}

func (p *IpOptimization) AddDomain(domainList ...string) *IpOptimization {
	p.domainList = append(p.domainList, domainList...)
	return p
}

func (p *IpOptimization) Exec() error {
	pterm.Info.Printfln("正在检查hosts文件")
	var err error
	p.hosts, err = txeh.NewHostsDefault()
	if err != nil {
		pterm.Error.Printfln("打开hosts文件失败(%s)", p.hosts.WriteFilePath)
		return err
	}

	err = p.hosts.Save()
	if err != nil {
		pterm.Error.Printfln("无法写入hosts文件，请检查权限(%s)", p.hosts.WriteFilePath)
		return err
	}
	pterm.Info.Printfln("hosts文件读写正常")

	p.nameserverList = p.nameserverList.Unique()
	p.domainList = p.domainList.Unique()

	p.hosts.RemoveAddresses(p.domainList)

	p.dnsOptimization()

	table := pterm.TableData{
		{
			"domain",
			"ip",
			"delay",
		},
	}

	failMap := map[string]bool{}
	p.incrBy(len(p.domainList) * len(p.nameserverList))
	p.domainList.Each(func(domain string) {
		err := func() error {
			ips, err := p.queryAllIps(domain)
			if err != nil {
				log.Errorf("err:%v", err)
				return err
			}

			if len(ips) <= 0 {
				pterm.Debug.Printfln("not found ips for %s", domain)
				return errors.New("not found any ips")
			}

			p.incrBy(len(ips))

			ip, delay, err := p.ipOptimization(domain, ips)
			if err != nil {
				log.Errorf("err:%v", err)
				pterm.Error.Printfln("%s %s", domain, err.Error())
				return err
			}

			table = append(table, []string{
				domain,
				ip,
				fmt.Sprintf("%s", delay),
			})

			pterm.Success.Printfln("%s\t%s\t%v", domain, ip, delay)
			p.hosts.AddHost(ip, domain)

			return nil
		}()
		if err != nil {
			log.Errorf("err:%v", err)
			failMap[domain] = true
			return
		}
	})

	if len(failMap) > 0 {
		pterm.Warning.Printfln("%d个域名处理失败，尝试从远端获取信息", len(failMap))
		resp, err := http.Get("https://raw.hellogithub.com/hosts.json")
		if err != nil {
			log.Errorf("err:%v", err)
			pterm.Warning.Printfln("获取远端hosts文件失败")
			for _, s := range failMap {
				pterm.Error.Printfln("%s 检测失败", s)
			}
		} else {
			buf, err := ioutil.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if err != nil {
				log.Errorf("err:%v", err)
				pterm.Warning.Printfln("读取远端hosts文件失败")
				for s := range failMap {
					pterm.Error.Printfln("%s 检测失败", s)
				}
			} else {
				var rsp [][]string
				err = json.Unmarshal(buf, &rsp)
				if err != nil {
					log.Errorf("err:%v", err)
					pterm.Warning.Printfln("解析远端hosts文件失败")
					for s := range failMap {
						pterm.Error.Printfln("%s 检测失败", s)
					}
				} else {
					for _, s := range rsp {
						if len(s) != 2 {
							continue
						}

						// 没有匹配上
						if !failMap[s[1]] {
							continue
						}

						delete(failMap, s[1])

						table = append(table, []string{
							s[1],
							s[0],
							"",
						})

						pterm.Success.Printfln("%s\t%s", s[1], s[0])
						p.hosts.AddHost(s[0], s[1])
					}
				}
			}
		}
	}

	pterm.Success.Printfln("检测完成(%v)", time.Since(p.start))
	_ = pterm.DefaultTable.WithBoxed(true).WithHasHeader(true).WithData(table).Render()

	_, _ = p.bar.Stop()

	pterm.Info.Printfln("正在尝试写入hosts")
	err = p.hosts.Save()
	if err != nil {
		log.Errorf("err:%v", err)
		pterm.Error.Printfln("打开hosts文件失败(%s)", p.hosts.WriteFilePath)
		return err
	}
	pterm.Success.Printfln("更新hosts成功")
	pterm.Info.Printfln("检测耗时%v", time.Since(p.start))
	return nil
}

func (p *IpOptimization) ipOptimization(domain string, ips pie.Strings) (string, time.Duration, error) {
	direct := outbound.NewDirect()

	var fastIp string
	fastDelay := time.Duration(-1)
	ips.Each(func(ip string) {
		p.increment("check %s", domain)

		dial, err := direct.DialContext(context.TODO(), &constant.Metadata{
			NetWork: constant.TCP,
			Type:    constant.HTTPCONNECT,
			DstIP:   net.ParseIP(ip),
			DstPort: "443",
			Host:    domain,
			DNSMode: constant.DNSMapping,
		})
		if err != nil {
			log.Errorf("err:%v", err)
			return
		}
		defer dial.Close()

		client := http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					return dial, nil
				},
				TLSHandshakeTimeout:   time.Second * 30,
				DisableKeepAlives:     true,
				DisableCompression:    true,
				MaxIdleConns:          1,
				MaxIdleConnsPerHost:   1,
				MaxConnsPerHost:       1,
				IdleConnTimeout:       time.Second * 30,
				ResponseHeaderTimeout: time.Second * 30,
				ExpectContinueTimeout: time.Second * 30,
			},
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
			Timeout: time.Second * 10,
		}

		start := time.Now()
		_, err = client.Head("https://" + domain)
		delay := time.Since(start)
		if err != nil {
			log.Errorf("err:%v", err)
			pterm.Debug.Printfln("%s %s fail", domain, ip)
			return
		}

		if fastDelay < 0 || fastDelay > delay {
			fastDelay = delay
			fastIp = ip
		}
	})

	if fastDelay < 0 {
		return "", -1, errors.New("no usable ip")
	}

	return fastIp, fastDelay, nil
}

func (p *IpOptimization) queryAllIps(domain string) (pie.Strings, error) {
	var ipList pie.Strings
	p.nameserverList.Each(func(s string) {
		ips, err := p.dnsQuery(s, domain)
		if err != nil {
			log.Errorf("err:%v", err)
			return
		}

		for _, ip := range ips {
			ipList = append(ipList, ip.String())
		}
	})

	return ipList.Unique(), nil
}

func (p *IpOptimization) dnsOptimization() {
	pterm.Info.Printfln("正在挑选合适的检测服务器")
	defer pterm.Success.Printfln("挑选完成(%d)", len(p.nameserverList))

	p.incrBy(len(p.nameserverList))

	var list []string
	p.nameserverList.Each(func(s string) {
		_, err := p.dnsQuery(s, "baidu.com")
		if err != nil {
			log.Errorf("err:%v", err)
			return
		}

		list = append(list, s)
	})

	p.nameserverList = list
}

func (p *IpOptimization) dnsQuery(nameserver, domain string) ([]net.IPAddr, error) {
	p.increment("query %s", domain)

	var client *net.Resolver
	var err error
	if utils.IsIp(nameserver) {
		client, err = dns.NewDoTResolver(strings.TrimSuffix(nameserver, ":53") + ":53")
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

	cnameMap := map[string]bool{
		domain: true,
	}
	for {
		cname, err := client.LookupCNAME(context.TODO(), domain)
		if err != nil {
			log.Errorf("err:%v", err)
			break
		}
		if cname == "" {
			break
		}

		cname = strings.TrimSuffix(cname, ".")
		if cnameMap[cname] {
			break
		}

		if domain == cname {
			break
		}

		pterm.Debug.Printfln("%s cname %s (%s)", domain, cname, nameserver)
		domain = cname
		cnameMap[cname] = true
	}

	ips, err := client.LookupIPAddr(context.TODO(), domain)
	if err != nil {
		log.Errorf("err:%v", err)
		pterm.Debug.Printfln("%s not found ip (%s)", domain, nameserver)
		return nil, err
	}

	return ips, nil
}
