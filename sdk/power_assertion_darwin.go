//go:build darwin

package sdk

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"sync"
)

type powerAssertion struct {
	cancel context.CancelFunc
	done   chan struct{}
	once   sync.Once
}

func startPowerAssertion(logf func(string, ...interface{})) *powerAssertion {
	if _, err := exec.LookPath("caffeinate"); err != nil {
		logf("macOS idle sleep assertion unavailable: %v", err)
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	assertion := &powerAssertion{
		cancel: cancel,
		done:   make(chan struct{}),
	}

	cmd := exec.CommandContext(ctx, "caffeinate", "-i", "-w", strconv.Itoa(os.Getpid()))
	if err := cmd.Start(); err != nil {
		cancel()
		logf("failed to start macOS idle sleep assertion: %v", err)
		return nil
	}

	go func() {
		defer close(assertion.done)
		if err := cmd.Wait(); ctx.Err() == nil && err != nil {
			logf("macOS idle sleep assertion stopped unexpectedly: %v", err)
		}
	}()

	logf("macOS idle sleep assertion active while VPN is connected")
	return assertion
}

func (p *powerAssertion) Stop() {
	if p == nil {
		return
	}
	p.once.Do(func() {
		p.cancel()
		<-p.done
	})
}
