package privileged

// MaxIPCMessageBytes bounds one newline-delimited privileged request or
// response. The client rejects oversized requests before writing them, and
// the broker/worker scanners reject oversized inbound frames before decoding.
// Privileged payloads are intentionally small; larger messages are rejected
// before they can grow broker memory without bound.
const MaxIPCMessageBytes = 1 << 20
