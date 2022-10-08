package main

import (
	_ "embed"
	"strings"

	"github.com/elliotchance/pie/pie"
)

//go:embed dns.fuck
var dnsBuf string
var dnsList pie.Strings

func init() {
	dnsList = pie.Strings(strings.Split(dnsBuf, "\n")).
			FilterNot(func(s string) bool {
				return s == ""
			}).
		Unique()
}
