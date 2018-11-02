package main

/* vim: set ts=2 sw=2 sts=2 ff=unix ft=go noet: */

import (
	"flag"
	"os"

	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
)

var (
	cfgDebugMode bool
	cfgHost      string
	cfgUsername  string
	log          = logrus.New()
)

func init() {
	flag.BoolVar(&cfgDebugMode, "v", false, "enable debug mode")
	flag.StringVar(&cfgHost, "h", "127.0.0.1:8000", "server host and port")
	flag.StringVar(&cfgUsername, "u", "", "client username")
	flag.Parse()

	if cfgDebugMode {
		log.SetLevel(logrus.DebugLevel)
	} else {
		log.SetLevel(logrus.WarnLevel)
	}
}

func main() {
	ctx := SignalContext(context.Background())

	DebugLogf("Connecting to server [%s]...", cfgHost)
	err := NewClient(cfgHost, cfgUsername).Run(ctx)
	if err != nil {
		ErrorLogf("Client terminated with error: %v", err)
		os.Exit(1)
	}
}
