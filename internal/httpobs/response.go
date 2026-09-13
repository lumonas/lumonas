// Package httpobs contains small HTTP observability primitives shared by the
// appliance API without changing the standard net/http handler contract.
package httpobs

import "net/http"

// Response records the status code and response bytes written by a handler.
// It also preserves streaming responses by forwarding http.Flusher.
type Response struct {
	http.ResponseWriter
	status int
	bytes  int
}

// Wrap observes a response while retaining the underlying writer's headers.
func Wrap(writer http.ResponseWriter) *Response {
	return &Response{ResponseWriter: writer}
}

func (w *Response) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *Response) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	count, err := w.ResponseWriter.Write(data)
	w.bytes += count
	return count, err
}

// Flush keeps Server-Sent Events and other streaming handlers working through
// the observer. A flush implicitly commits a successful response.
func (w *Response) Flush() {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Status returns the committed status, defaulting to the net/http success
// status when a handler did not write a response body.
func (w *Response) Status() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

// Bytes returns the number of response body bytes written by the handler.
func (w *Response) Bytes() int { return w.bytes }
