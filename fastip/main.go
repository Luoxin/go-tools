package main

import (
	"github.com/darabuchi/log"
	"github.com/pterm/pterm"
)

func main() {
	log.SetLevel(log.FatalLevel)

	// pterm.EnableDebugMessages()
	pterm.EnableColor()
	pterm.EnableOutput()
	pterm.EnableStyling()

	err := NewIpOptimization().AddNameService(nameserverList...).AddDomain(domainList...).Exec()
	if err != nil {
		log.Errorf("err:%v", err)
		return
	}
}
