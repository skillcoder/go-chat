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
	cfgListen    string
	log          = logrus.New()
)

func init() {
	flag.BoolVar(&cfgDebugMode, "v", false, "enable debug mode")
	flag.StringVar(&cfgListen, "l", "0.0.0.0:8000", "listen interface and port")
	flag.Parse()

	log.SetOutput(os.Stdout)
	if cfgDebugMode {
		log.SetLevel(logrus.DebugLevel)
	} else {
		log.SetLevel(logrus.WarnLevel)
	}
}

func main() {
	ctx := SignalContext(context.Background())

	DebugLogf("Running server...")
	err := NewServer(cfgListen).Run(ctx)
	if err != nil {
		ErrorLogf("Server terminated with error: %v", err)
		os.Exit(1)
	}
}
