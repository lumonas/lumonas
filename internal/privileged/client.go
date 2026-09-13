package privileged

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/lumonas/lumonas/internal/trace"
)

type Request struct {
	Operation        string            `json:"operation"`
	CorrelationID    string            `json:"correlationId,omitempty"`
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
	GPTDiskGUID    string `json:"gptDiskGuid,omitempty"`
	PartitionUUID  string `json:"partitionUuid,omitempty"`
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

const DefaultTimeout = 30 * time.Second

func (c Client) Execute(ctx context.Context, request Request) (Response, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, DefaultTimeout)
		defer cancel()
	}
	if request.CorrelationID == "" {
		request.CorrelationID = trace.CorrelationID(ctx)
	}
	if request.CorrelationID == "" {
		request.CorrelationID = request.OperationID
	}
	if request.CorrelationID == "" {
		request.CorrelationID = trace.NewCorrelationID()
	}
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
	if deadline, ok := ctx.Deadline(); ok {
		if err := connection.SetDeadline(deadline); err != nil {
			return Response{}, fmt.Errorf("set privileged request deadline: %w", err)
		}
	}
	stopCancellation := context.AfterFunc(ctx, func() {
		_ = connection.SetDeadline(time.Now())
	})
	defer stopCancellation()
	payload, err := json.Marshal(request)
	if err != nil {
		return Response{}, fmt.Errorf("encode privileged request: %w", err)
	}
	if len(payload)+1 > MaxIPCMessageBytes {
		return Response{}, fmt.Errorf("privileged request exceeds %d bytes", MaxIPCMessageBytes)
	}
	payload = append(payload, '\n')
	written, err := connection.Write(payload)
	if err != nil {
		return Response{}, fmt.Errorf("send privileged request: %w", err)
	}
	if written != len(payload) {
		return Response{}, io.ErrShortWrite
	}
	var response Response
	if err := json.NewDecoder(io.LimitReader(connection, MaxIPCMessageBytes)).Decode(&response); err != nil {
		return Response{}, fmt.Errorf("read privileged response: %w", err)
	}
	return response, nil
}
