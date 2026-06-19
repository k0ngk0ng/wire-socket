//go:build !darwin

package sdk

type powerAssertion struct{}

func startPowerAssertion(logf func(string, ...interface{})) *powerAssertion {
	return nil
}

func (p *powerAssertion) Stop() {}
