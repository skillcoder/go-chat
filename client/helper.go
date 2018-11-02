package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/net/context"
)

const timeFormat = "15:04:05"

// DebugLogf - Send to log debug info
func DebugLogf(format string, args ...interface{}) {
	if !cfgDebugMode {
		return
	}
	log.Debugf("[%s] "+format, append([]interface{}{time.Now().Format(timeFormat)}, args...)...)
}

// WarnLogf - Send to log warning
func WarnLogf(format string, args ...interface{}) {
	log.Warnf("[%s] S> "+format, append([]interface{}{time.Now().Format(timeFormat)}, args...)...)
}

// ErrorLogf - Send to log error
func ErrorLogf(format string, args ...interface{}) {
	log.Errorf("[%s] "+format, append([]interface{}{time.Now().Format(timeFormat)}, args...)...)
}

// SignalContext - Monitoring and handle os signals by cancel ctx
func SignalContext(ctx context.Context) context.Context {
	ctx, cancel := context.WithCancel(ctx)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		DebugLogf("listening for shutdown signal")
		<-sigs
		DebugLogf("shutdown signal received")
		signal.Stop(sigs)
		close(sigs)
		cancel()
	}()

	return ctx
}
