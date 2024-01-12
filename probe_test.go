package probe_test

import (
	"context"
	"github.com/zxfishhack/probe"
	"gotest.tools/v3/assert"
	"testing"
	"time"
)

func TestProbe(t *testing.T) {
	l, err := probe.NewListener(context.Background(), "1")
	assert.NilError(t, err)
	go l.Serve()
	ctx, cancel := context.WithCancel(context.Background())
	res, err := probe.Probe(ctx, "2")
	timer := time.NewTimer(5 * time.Minute)
	running := true
	for running {
		select {
		case r := <-res:
			t.Logf("%#v", r)
		case <-timer.C:
			running = false
		}
	}
	cancel()
	l.Stop()
}
