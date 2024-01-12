package main

import (
	"context"
	"flag"
	"github.com/zxfishhack/probe"
	"log"
	"os"
	"os/signal"
)

func main() {
	var listen bool
	var id string
	flag.BoolVar(&listen, "l", false, "start listen mode")
	flag.StringVar(&id, "i", "", "probe id")
	flag.Parse()
	if id == "" {
		flag.PrintDefaults()
		return
	}
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	if listen {
		l, err := probe.NewListener(context.Background(), id)
		if err != nil {
			log.Fatal(err)
		}
		go func() {
			for range c {
				l.Stop()
			}
		}()
		err = l.Serve()
		if err != nil {
			log.Print(err)
		}
	} else {
		ctx, cancel := context.WithCancel(context.Background())
		resC, err := probe.Probe(ctx, id)
		if err != nil {
			log.Fatal(err)
		}
		go func() {
			for range c {
				cancel()
			}
		}()
		for res := range resC {
			log.Printf("%#v", res)
		}
	}
}
