package main

import (
	"testing"
)

func TestNameserverCheck(t *testing.T) {
	type args struct {
		nameserver string
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "udp-v4",
			args: args{
				nameserver: "223.5.5.5",
			},
		},
		{
			name: "udp-v6",
			args: args{
				nameserver: "2400:3200::1",
			},
		},
		{
			name: "dot",
			args: args{
				nameserver: "tls://dns.alidns.com",
			},
		},
		{
			name: "dot-port",
			args: args{
				nameserver: "tls://185.184.222.222:853",
			},
		},
		{
			name: "http",
			args: args{
				nameserver: "https://223.5.5.5/dns-query",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check := NewNameserverCheck(tt.args.nameserver)
			check.Check()
			t.Logf("%v check result %+v", tt.name, check)
		})
	}
}
