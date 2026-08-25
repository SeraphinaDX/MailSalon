package transport

import (
	"context"
	"strings"
	"testing"
)

func TestReceiveAndSend(t *testing.T) {
	out, err := Receive(context.Background(), `printf 'synced'`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "synced" {
		t.Fatalf("receive output = %q", out)
	}

	out, err = Send(context.Background(), `cat`, []byte("message body"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "message body" {
		t.Fatalf("send stdin was not piped: %q", out)
	}
}
