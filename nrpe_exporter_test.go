package main

import (
	"net"
	"testing"

	"github.com/aperum/nrpe"
	"github.com/go-kit/kit/log"
)

func TestClientServer(t *testing.T) {
	sock := testCreateSocketPair(t)
	defer sock.Close()
	c := make(chan float64)

	go func() {
		err := nrpe.ServeOne(sock.server, func(command nrpe.Command) (*nrpe.CommandResult, error) {
			return &nrpe.CommandResult{
				StatusLine: (""),
				StatusCode: nrpe.StatusOK,
			}, nil
		}, false, 0)

		if err != nil {
			t.Fatal(err)
		}

		c <- 1.0
	}()

	cr, err := collectCommandMetrics("check_load", sock.client, log.NewNopLogger())
	if err != nil {
		t.Fatal(err)
	}

	nrpeResult := nrpe.CommandResult{StatusLine: "", StatusCode: 0.0}
	expectedResults := CommandResult{0.0, 1.0, &nrpeResult}
	if cr.commandDuration < expectedResults.commandDuration {
		t.Fatalf("Expected commandDuration greater than 0s, got: %v", cr.commandDuration)
	}
	if cr.statusOk != expectedResults.statusOk {
		t.Fatalf("Expected %v, got: %v", expectedResults.statusOk, cr.statusOk)
	}
	if cr.result.StatusCode != expectedResults.result.StatusCode {
		t.Fatalf("Expected %v, got: %v", expectedResults.result.StatusCode, cr.result.StatusCode)
	}
}

type testSocketPair struct {
	client net.Conn
	server net.Conn
}

func (s *testSocketPair) Close() {
	s.client.Close()
	s.server.Close()
}

func testCreateSocketPair(t *testing.T) *testSocketPair {
	serverConn, clientConn := net.Pipe()
	s := &testSocketPair{
		client: clientConn,
		server: serverConn,
	}
	return s
}
