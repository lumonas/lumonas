package privileged

// MaxIPCMessageBytes bounds one newline-delimited privileged request or
// response. Privileged payloads are intentionally small; larger messages are
// rejected before they can grow broker memory without bound.
const MaxIPCMessageBytes = 1 << 20
