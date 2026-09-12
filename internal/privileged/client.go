package privileged

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"
)

type Request struct {
	Operation        string            `json:"operation"`
	OperationID      string            `json:"operationId,omitempty"`
	PlanHash         string            `json:"planHash"`
	TargetDiskID     string            `json:"targetDiskId,omitempty"`
	ExpectedIdentity map[string]string `json:"expectedIdentity,omitempty"`
	ExpectedDisks    []ExpectedDisk    `json:"expectedDisks,omitempty"`
	ExpectedState    map[string]string `json:"expectedState,omitempty"`
	RequestedState   map[string]any    `json:"requestedState,omitempty"`
	ExpiresAt        time.Time         `json:"expiresAt,omitempty"`
	Confirmed        bool              `json:"confirmed"`
}

type ExpectedDisk struct {
	ID             string `json:"id"`
	WWN            string `json:"wwn,omitempty"`
	Serial         string `json:"serial,omitempty"`
	Model          string `json:"model,omitempty"`
	SizeBytes      uint64 `json:"sizeBytes"`
	FilesystemUUID string `json:"filesystemUuid,omitempty"`
}

type Response struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	Data  any    `json:"data,omitempty"`
}

type Dialer interface {
	DialContext(context.Context, string, string) (net.Conn, error)
}

type Client struct {
	Socket string
	Dialer Dialer
}

func (c Client) Execute(ctx context.Context, request Request) (Response, error) {
	if c.Socket == "" {
		c.Socket = "/run/lumonas/privd.sock"
	}
	dialer := c.Dialer
	if dialer == nil {
		dialer = &net.Dialer{Timeout: 3 * time.Second}
	}
	connection, err := dialer.DialContext(ctx, "unix", c.Socket)
	if err != nil {
		return Response{}, fmt.Errorf("connect privileged broker: %w", err)
	}
	defer connection.Close()
	if err := json.NewEncoder(connection).Encode(request); err != nil {
		return Response{}, fmt.Errorf("send privileged request: %w", err)
	}
	var response Response
	if err := json.NewDecoder(bufio.NewReader(connection)).Decode(&response); err != nil {
		return Response{}, fmt.Errorf("read privileged response: %w", err)
	}
	return response, nil
}
