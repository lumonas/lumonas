package virtualization

import (
	"context"
	"errors"
	fmt "fmt"
	"strings"
)

const MaxConsoleInputBytes = 4096

type ConsoleState struct {
	Output    string `json:"output"`
	Cursor    int64  `json:"cursor"`
	Connected bool   `json:"connected"`
}

// ConsoleRuntime owns interactive serial processes independently of HTTP
// requests. Keeping the interface injectable lets the API exercise access and
// lifecycle behavior without a host libvirt installation.
type ConsoleRuntime interface {
	Start(context.Context, string, string) error
	Read(string, int64) (ConsoleState, error)
	Write(string, []byte) error
	Close(string) error
}

func (s Service) OpenConsole(ctx context.Context, name string) error {
	if !domainNamePattern.MatchString(name) {
		return errors.New("VM name is invalid")
	}
	if s.ConsoleRuntime == nil {
		return errors.New("serial console is unavailable on this platform")
	}
	state, err := s.command(ctx, "domstate", name)
	if err != nil {
		return fmt.Errorf("check VM state before opening its serial console: %w", err)
	}
	if strings.TrimSpace(strings.ToLower(string(state))) != "running" {
		return errors.New("VM must be running before its serial console can be opened")
	}
	virsh := s.Virsh
	if virsh == "" {
		virsh = "virsh"
	}
	return s.ConsoleRuntime.Start(ctx, virsh, name)
}

func (s Service) ReadConsole(name string, cursor int64) (ConsoleState, error) {
	if !domainNamePattern.MatchString(name) || cursor < 0 {
		return ConsoleState{}, errors.New("VM name or console cursor is invalid")
	}
	if s.ConsoleRuntime == nil {
		return ConsoleState{}, errors.New("serial console is unavailable on this platform")
	}
	return s.ConsoleRuntime.Read(name, cursor)
}

func (s Service) WriteConsole(name string, data []byte) error {
	if !domainNamePattern.MatchString(name) {
		return errors.New("VM name is invalid")
	}
	if len(data) == 0 || len(data) > MaxConsoleInputBytes {
		return fmt.Errorf("console input must be between 1 and %d bytes", MaxConsoleInputBytes)
	}
	if s.ConsoleRuntime == nil {
		return errors.New("serial console is unavailable on this platform")
	}
	return s.ConsoleRuntime.Write(name, data)
}

func (s Service) CloseConsole(name string) error {
	if !domainNamePattern.MatchString(name) {
		return errors.New("VM name is invalid")
	}
	if s.ConsoleRuntime == nil {
		return nil
	}
	return s.ConsoleRuntime.Close(name)
}
