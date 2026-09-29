//go:build !linux

package virtualization

import (
	"context"
	"errors"
)

type unavailableConsoleRuntime struct{}

func newConsoleRuntime() ConsoleRuntime { return unavailableConsoleRuntime{} }
func (unavailableConsoleRuntime) Start(context.Context, string, string) error {
	return errors.New("serial console is supported only on Linux hosts")
}
func (unavailableConsoleRuntime) Read(string, int64) (ConsoleState, error) {
	return ConsoleState{}, errors.New("serial console is supported only on Linux hosts")
}
func (unavailableConsoleRuntime) Write(string, []byte) error {
	return errors.New("serial console is supported only on Linux hosts")
}
func (unavailableConsoleRuntime) Close(string) error { return nil }
