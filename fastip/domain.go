package main

import (
	_ "embed"
	"strings"

	"github.com/elliotchance/pie/pie"
)

//go:embed domain.fuck
var domainBuf string
var domainList pie.Strings

func init() {
	domainList = pie.Strings(strings.Split(domainBuf, "\n")).
			FilterNot(func(s string) bool {
				return s == ""
			}).
		Unique()
}
