//go:build linux

package virtualization

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

const (
	consoleBufferBytes  = 256 << 10
	consoleReadBytes    = 32 << 10
	consoleIdleLimit    = 30 * time.Minute
	consoleSessionLimit = 4
)

type linuxConsoleRuntime struct {
	mu            sync.Mutex
	sessions      map[string]*linuxConsoleSession
	reaperStarted bool
}

type linuxConsoleSession struct {
	mu           sync.Mutex
	writeMu      sync.Mutex
	master       *os.File
	command      *exec.Cmd
	output       []byte
	baseCursor   int64
	connected    bool
	lastActivity time.Time
}

func newConsoleRuntime() ConsoleRuntime {
	return &linuxConsoleRuntime{sessions: make(map[string]*linuxConsoleSession)}
}

func (runtime *linuxConsoleRuntime) Start(ctx context.Context, virsh, domain string) error {
	if !domainNamePattern.MatchString(domain) || virsh == "" {
		return errors.New("VM serial console request is invalid")
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if current := runtime.sessions[domain]; current != nil {
		current.mu.Lock()
		active := current.connected && time.Since(current.lastActivity) < consoleIdleLimit
		current.lastActivity = time.Now()
		current.mu.Unlock()
		if active {
			return nil
		}
		current.stop()
		delete(runtime.sessions, domain)
	}
	if len(runtime.sessions) >= consoleSessionLimit {
		return errors.New("serial console session limit reached; close an existing console first")
	}
	master, slave, err := openPTY()
	if err != nil {
		return fmt.Errorf("open VM console PTY: %w", err)
	}
	command := exec.Command(virsh, "console", domain, "--force")
	command.Stdin, command.Stdout, command.Stderr = slave, slave, slave
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := command.Start(); err != nil {
		_ = master.Close()
		_ = slave.Close()
		return fmt.Errorf("start libvirt serial console: %w", err)
	}
	_ = slave.Close()
	session := &linuxConsoleSession{master: master, command: command, connected: true, lastActivity: time.Now()}
	runtime.sessions[domain] = session
	if !runtime.reaperStarted {
		runtime.reaperStarted = true
		go runtime.reapIdleSessions()
	}
	go session.readLoop()
	go func() {
		_ = command.Wait()
		session.mu.Lock()
		session.connected = false
		session.mu.Unlock()
		_ = master.Close()
	}()
	return nil
}

func (runtime *linuxConsoleRuntime) reapIdleSessions() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		runtime.mu.Lock()
		var expired []*linuxConsoleSession
		for name, session := range runtime.sessions {
			session.mu.Lock()
			idle := now.Sub(session.lastActivity) >= consoleIdleLimit
			finished := !session.connected && now.Sub(session.lastActivity) >= time.Minute
			session.mu.Unlock()
			if idle || finished {
				delete(runtime.sessions, name)
				expired = append(expired, session)
			}
		}
		runtime.mu.Unlock()
		for _, session := range expired {
			session.stop()
		}
	}
}

func openPTY() (*os.File, *os.File, error) {
	masterFD, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}
	closeMaster := true
	defer func() {
		if closeMaster {
			_ = unix.Close(masterFD)
		}
	}()
	if err := unix.IoctlSetPointerInt(masterFD, unix.TIOCSPTLCK, 0); err != nil {
		return nil, nil, err
	}
	ptyNumber, err := unix.IoctlGetInt(masterFD, unix.TIOCGPTN)
	if err != nil {
		return nil, nil, err
	}
	slaveFD, err := unix.Open(filepath.Join("/dev/pts", fmt.Sprint(ptyNumber)), unix.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, nil, err
	}
	closeMaster = false
	return os.NewFile(uintptr(masterFD), "lumonas-vm-console-master"), os.NewFile(uintptr(slaveFD), "lumonas-vm-console-slave"), nil
}

func (session *linuxConsoleSession) readLoop() {
	chunk := make([]byte, 8192)
	for {
		count, err := session.master.Read(chunk)
		if count > 0 {
			session.mu.Lock()
			session.output = append(session.output, chunk[:count]...)
			if excess := len(session.output) - consoleBufferBytes; excess > 0 {
				session.output = append([]byte(nil), session.output[excess:]...)
				session.baseCursor += int64(excess)
			}
			session.lastActivity = time.Now()
			session.mu.Unlock()
		}
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, os.ErrClosed) {
				// PTYs report EIO when the child closes the slave side.
				if !errors.Is(err, syscall.EIO) {
					session.mu.Lock()
					session.connected = false
					session.mu.Unlock()
				}
			}
			return
		}
	}
}

func (runtime *linuxConsoleRuntime) Read(domain string, cursor int64) (ConsoleState, error) {
	session := runtime.session(domain)
	if session == nil {
		return ConsoleState{}, errors.New("serial console is not open")
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if cursor < session.baseCursor {
		cursor = session.baseCursor
	}
	end := session.baseCursor + int64(len(session.output))
	if cursor > end {
		cursor = end
	}
	start := int(cursor - session.baseCursor)
	stop := start + consoleReadBytes
	if stop > len(session.output) {
		stop = len(session.output)
	}
	session.lastActivity = time.Now()
	return ConsoleState{Output: string(session.output[start:stop]), Cursor: session.baseCursor + int64(stop), Connected: session.connected}, nil
}

func (runtime *linuxConsoleRuntime) Write(domain string, data []byte) error {
	session := runtime.session(domain)
	if session == nil {
		return errors.New("serial console is not open")
	}
	session.writeMu.Lock()
	defer session.writeMu.Unlock()
	if _, err := session.master.Write(data); err != nil {
		return fmt.Errorf("write VM serial console input: %w", err)
	}
	session.mu.Lock()
	session.lastActivity = time.Now()
	session.mu.Unlock()
	return nil
}

func (runtime *linuxConsoleRuntime) Close(domain string) error {
	runtime.mu.Lock()
	session := runtime.sessions[domain]
	delete(runtime.sessions, domain)
	runtime.mu.Unlock()
	if session == nil {
		return nil
	}
	session.stop()
	return nil
}

func (runtime *linuxConsoleRuntime) session(domain string) *linuxConsoleSession {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return runtime.sessions[domain]
}

func (session *linuxConsoleSession) stop() {
	session.mu.Lock()
	session.connected = false
	session.mu.Unlock()
	_ = session.master.Close()
	if session.command.Process != nil {
		_ = session.command.Process.Kill()
	}
}
