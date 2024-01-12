package probe

import (
	"context"
	"encoding/json"
	"errors"
	"golang.org/x/sync/errgroup"
	"log"
	"net"
	"time"
)

var Port = 9394

var (
	multicastIP = net.ParseIP("224.0.0.1")
)

type Listener interface {
	Serve() error
	Stop()
}

type Result struct {
	ID string
	IP string
}

type pkg struct {
	msg  message
	addr net.Addr
}

type Prober struct {
	id     string
	conn   []*net.UDPConn
	addr   *net.UDPAddr
	g      *errgroup.Group
	ctx    context.Context
	cancel func()
	msg    chan *pkg
	result chan Result
}

func (l *Prober) readLoop(conn *net.UDPConn) (err error) {
	for {
		buf := make([]byte, 1500)
		p := &pkg{}
		_, p.addr, err = conn.ReadFrom(buf)
		if err != nil {
			return
		}
		log.Print(string(buf))
		err = json.Unmarshal(buf, &p.msg)
		if err == nil {
			l.msg <- p
		}
	}
}

func (l *Prober) startRead() {
	for _, conn := range l.conn {
		l.g.Go(func() error {
			return l.readLoop(conn)
		})
	}
}

func (l *Prober) stopRead() {
	for _, conn := range l.conn {
		_ = conn.Close()
	}
}

func (l *Prober) mainLoop() (err error) {
	defer l.stopRead()
	for {
		select {
		case <-l.ctx.Done():
			err = l.ctx.Err()
			if errors.Is(err, context.Canceled) {
				err = nil
			}
			return
		case m := <-l.msg:
			log.Printf("%#v", m)
		}
	}
}

func (l *Prober) probeLoop() (err error) {
	defer l.stopRead()
	timer := time.NewTimer(time.Second)
	for {
		select {
		case <-l.ctx.Done():
			err = l.ctx.Err()
			if errors.Is(err, context.Canceled) {
				err = nil
			}
			return
		case m := <-l.msg:
			log.Printf("%#v", m)
		case <-timer.C:
			for _, conn := range l.conn {
				_, e := conn.WriteTo([]byte("xxx"), l.addr)
				log.Print(e, *l.addr)
			}
			timer.Reset(30 * time.Second)
		}
	}
}

func (l *Prober) probe() {
	l.result = make(chan Result, 10)
	l.startRead()
	l.g.Go(l.probeLoop)
}

func (l *Prober) Serve() (err error) {
	l.startRead()
	l.g.Go(l.mainLoop)
	return l.g.Wait()
}

func (l *Prober) Stop() {
	l.cancel()
}

func newProber(ctx context.Context, id string) (o *Prober, err error) {
	o = &Prober{
		id:   id,
		msg:  make(chan *pkg, 10),
		addr: &net.UDPAddr{IP: multicastIP, Port: Port},
	}
	ifs, err := net.Interfaces()
	if err != nil {
		return
	}
	for _, ifi := range ifs {
		var c *net.UDPConn
		if ifi.Flags&net.FlagLoopback == net.FlagLoopback {
			continue
		}
		c, err = net.ListenMulticastUDP("udp4", &ifi, o.addr)
		if err != nil {
			continue
		}
		log.Printf("start prober @ %s", ifi.Name)
		o.conn = append(o.conn, c)
	}
	if err != nil {
		return
	}
	o.ctx, o.cancel = context.WithCancel(ctx)
	o.g, o.ctx = errgroup.WithContext(o.ctx)

	return
}

func NewListener(ctx context.Context, id string) (l Listener, err error) {
	l, err = newProber(ctx, id)
	return
}

func Probe(ctx context.Context, id string) (resC <-chan Result, err error) {
	l, err := newProber(ctx, id)
	if err != nil {
		return
	}
	l.probe()
	resC = l.result
	return
}
