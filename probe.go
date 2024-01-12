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

type connector struct {
	*net.UDPConn
	ifi net.Interface
}

type Result struct {
	ID string
	IP string
}

type Listener struct {
	id     string
	conn   []*connector
	addr   *net.UDPAddr
	g      *errgroup.Group
	ctx    context.Context
	result chan Result
}

func (l *Listener) readLoop(conn *connector) (err error) {
	var n int
	for {
		buf := make([]byte, conn.ifi.MTU)
		var msg message
		n, _, err = conn.ReadFrom(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				err = nil
			}
			if err != nil {
				log.Printf("ReadFrom %s failed %s", conn.ifi.Name, err.Error())
			}
			return
		}
		err = json.Unmarshal(buf[:n], &msg)
		if err == nil && msg.verify() == nil {
			msg.ID = l.id
			msg.signature()
			buf, err = json.Marshal(msg)
			_, err = conn.WriteTo(buf, l.addr)
		}
	}
}

func (l *Listener) Serve() (err error) {
	for _, conn := range l.conn {
		func(c *connector) {
			l.g.Go(func() error {
				return l.readLoop(c)
			})
		}(conn)
		// l.g.Go(func() error { return l.readLoop(conn) })
	}
	return l.g.Wait()
}

func (l *Listener) Stop() {
	for _, conn := range l.conn {
		_ = conn.Close()
	}
}

func NewListener(ctx context.Context, id string) (l *Listener, err error) {
	o := &Listener{
		id:   id,
		ctx:  ctx,
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
			err = nil
			continue
		}
		log.Printf("start listener @ %s", ifi.Name)
		o.conn = append(o.conn, &connector{
			UDPConn: c,
			ifi:     ifi,
		})
	}
	if err != nil {
		return
	}
	o.g, o.ctx = errgroup.WithContext(o.ctx)
	l = o
	return
}

type prober struct {
	id     string
	conn   []*connector
	addr   *net.UDPAddr
	g      *errgroup.Group
	ctx    context.Context
	result chan Result
}

func (p *prober) readLoop(conn *connector) (err error) {
	var n int
	for {
		buf := make([]byte, 1500)
		var addr net.Addr
		var msg message
		n, addr, err = conn.ReadFrom(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				err = nil
			}
			if err != nil {
				log.Printf("ReadFrom %s failed %s", conn.ifi.Name, err.Error())
			}
			return
		}
		err = json.Unmarshal(buf[:n], &msg)
		if err == nil && msg.verify() == nil {
			p.result <- Result{
				ID: msg.ID,
				IP: addr.(*net.UDPAddr).IP.String(),
			}
		}
	}
}

func (p *prober) stopRead() {
	for _, conn := range p.conn {
		_ = conn.Close()
	}
}

func (p *prober) probeLoop() {
	defer close(p.result)
	defer p.g.Wait()
	defer p.stopRead()
	timer := time.NewTimer(time.Second)
	for {
		select {
		case <-p.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			var msg message
			msg.ID = p.id
			msg.signature()
			var buf []byte
			var err error
			buf, err = json.Marshal(msg)
			if err == nil {
				for _, conn := range p.conn {
					_, _ = conn.WriteTo(buf, p.addr)
				}
			}
			timer.Reset(30 * time.Second)
		}
	}
}

func newProbe(ctx context.Context, id string) (l *prober, err error) {
	o := &prober{
		id:     id,
		ctx:    ctx,
		result: make(chan Result, 10),
		addr:   &net.UDPAddr{IP: multicastIP, Port: Port},
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
			err = nil
			continue
		}
		log.Printf("start prober @ %s", ifi.Name)
		o.conn = append(o.conn, &connector{
			UDPConn: c,
			ifi:     ifi,
		})
	}
	if err != nil {
		return
	}
	o.g, o.ctx = errgroup.WithContext(o.ctx)
	l = o
	return
}

func Probe(ctx context.Context, id string) (resC <-chan Result, err error) {
	p, err := newProbe(ctx, id)
	if err != nil {
		return
	}
	for _, conn := range p.conn {
		func(c *connector) {
			p.g.Go(func() error {
				return p.readLoop(c)
			})
		}(conn)
	}
	go p.probeLoop()
	resC = p.result
	return
}
