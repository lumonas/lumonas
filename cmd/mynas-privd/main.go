package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

type request struct {
	Operation string `json:"operation"`
	PlanHash  string `json:"planHash"`
	DiskID    string `json:"diskId,omitempty"`
}
type response struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	Data  any    `json:"data,omitempty"`
}

func main() {
	socket := flag.String("socket", "/run/mynas/privd.sock", "Unix socket path")
	flag.Parse()
	_ = os.Remove(*socket)
	if err := os.MkdirAll(filepath.Dir(*socket), 0o750); err != nil {
		panic(err)
	}
	listener, err := net.Listen("unix", *socket)
	if err != nil {
		panic(err)
	}
	defer listener.Close()
	_ = os.Chmod(*socket, 0o660)
	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}
		go serve(conn)
	}
}

func serve(conn net.Conn) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)
	encoder := json.NewEncoder(conn)
	for scanner.Scan() {
		var req request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			_ = encoder.Encode(response{Error: "invalid request"})
			continue
		}
		if req.PlanHash == "" {
			_ = encoder.Encode(response{Error: "planHash is required"})
			continue
		}
		switch req.Operation {
		case "ping":
			_ = encoder.Encode(response{OK: true, Data: map[string]string{"service": "mynas-privd"}})
		case "disk.read-identities":
			_ = encoder.Encode(response{OK: true, Data: map[string]string{"status": "read-only collector delegated to mynasd"}})
		default:
			_ = encoder.Encode(response{Error: fmt.Sprintf("operation %q is not allow-listed", strings.TrimSpace(req.Operation))})
		}
	}
}
