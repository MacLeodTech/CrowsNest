package dockerguard

import (
	"net"
	"testing"
	"time"
)

func TestParseAllowFrom(t *testing.T) {
	got, err := ParseAllowFrom("10.0.0.0/8, 192.0.2.7 ,")
	if err != nil || len(got) != 2 || got[1].String() != "192.0.2.7/32" {
		t.Fatalf("ParseAllowFrom = %v, %v", got, err)
	}
	if got, err := ParseAllowFrom(""); err != nil || got != nil {
		t.Errorf("empty = %v, %v", got, err)
	}
	if _, err := ParseAllowFrom("not-an-ip"); err == nil {
		t.Error("a bad entry must be an error")
	}
}

func TestAllowFromClosesOtherClients(t *testing.T) {
	inner, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer inner.Close()
	deny, _ := ParseAllowFrom("10.9.9.9")
	l := AllowFrom(inner, deny, nil)
	go func() { _, _ = l.Accept() }()
	c, err := net.Dial("tcp", inner.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := c.Read(make([]byte, 1)); err == nil {
		t.Fatal("a client outside the list was kept open")
	}

	inner2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer inner2.Close()
	allow, _ := ParseAllowFrom("127.0.0.0/8")
	l2 := AllowFrom(inner2, allow, nil)
	done := make(chan net.Conn, 1)
	go func() { c, _ := l2.Accept(); done <- c }()
	c2, err := net.Dial("tcp", inner2.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()
	select {
	case got := <-done:
		if got == nil {
			t.Fatal("no connection accepted")
		}
		got.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("a client inside the list was not accepted")
	}
}
