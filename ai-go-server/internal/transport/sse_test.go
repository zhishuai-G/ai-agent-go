package transport

import (
	"errors"
	"strings"
	"testing"
)

func TestParseSSEAggregatesDataLines(t *testing.T) {
	input := ": heartbeat\r\ndata: first\r\ndata: second\r\nevent: message\r\n\r\ndata: last\n\n"
	var got []string
	err := ParseSSE(strings.NewReader(input), func(data []byte) error {
		got = append(got, string(data))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "first\nsecond" || got[1] != "last" {
		t.Fatalf("events = %#v", got)
	}
}

func TestParseSSEDiscardsPartialEventAtEOF(t *testing.T) {
	called := false
	err := ParseSSE(strings.NewReader("data: incomplete"), func([]byte) error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("partial event was dispatched")
	}
}

func TestParseSSEStopsOnCallbackError(t *testing.T) {
	want := errors.New("stop")
	err := ParseSSE(strings.NewReader("data: item\n\n"), func([]byte) error { return want })
	if !errors.Is(err, want) {
		t.Fatalf("ParseSSE() error = %v, want %v", err, want)
	}
}
