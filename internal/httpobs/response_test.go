package httpobs

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResponseRecordsStatusAndBytes(t *testing.T) {
	recorder := httptest.NewRecorder()
	observed := Wrap(recorder)
	if _, err := observed.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	observed.WriteHeader(http.StatusCreated)
	if observed.Status() != http.StatusOK || observed.Bytes() != 5 {
		t.Fatalf("unexpected response observation: status=%d bytes=%d", observed.Status(), observed.Bytes())
	}
}

func TestResponsePreservesFlush(t *testing.T) {
	recorder := httptest.NewRecorder()
	observed := Wrap(recorder)
	observed.Flush()
	if !recorder.Flushed || observed.Status() != http.StatusOK {
		t.Fatalf("flush was not forwarded: flushed=%v status=%d", recorder.Flushed, observed.Status())
	}
}

func TestResponseDefaultsToOKWithoutWrites(t *testing.T) {
	observed := Wrap(httptest.NewRecorder())
	if observed.Status() != http.StatusOK || observed.Bytes() != 0 {
		t.Fatalf("unexpected empty response observation: status=%d bytes=%d", observed.Status(), observed.Bytes())
	}
}
