package endpointproxy

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strconv"

	"github.com/n24-x/fe/internal/testing/modules/dnsforwarder"
)

// Instance is an endpoint.proxy.server module instance.
type Instance struct {
	listen string
	port   int
	dns    *dnsforwarder.Instance // resolved dependency: resolver for domain names
	ln     net.Listener
}

// DNS returns the resolved dns.forwarder instance this server uses as its
// domain resolver.
func (inst *Instance) DNS() *dnsforwarder.Instance { return inst.dns }

// Start implements fe.Instance: it opens the TCP listener and returns. The
// accept loop runs in a goroutine; Stop closes the listener, which makes
// Accept return an error and ends the loop.
func (inst *Instance) Start() error {
	ln, err := net.Listen("tcp", inst.listen+":"+strconv.Itoa(inst.port))
	if err != nil {
		return fmt.Errorf("endpoint.proxy.server: listen on %s:%d: %w", inst.listen, inst.port, err)
	}
	inst.ln = ln
	log.Printf("[endpoint.proxy.server] started (resolver_upstream=%s listen=%s:%d)", inst.dns.Upstream(), inst.listen, inst.port)
	go inst.acceptLoop(ln)
	return nil
}

// acceptLoop accepts client connections until the listener is closed.
func (inst *Instance) acceptLoop(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			// Listener closed by Stop (or a real accept failure): end the loop.
			return
		}
		go handleConnection(conn)
	}
}

// handleConnection echoes every line back to the client until it disconnects.
func handleConnection(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return // client disconnected or errored
		}
		if _, err := conn.Write([]byte(line)); err != nil {
			return
		}
	}
}

// Stop implements fe.Instance: it closes the listener, ending the accept
// loop. Closing twice is an error by net.Listener convention, but Stop is
// idempotent at the Runtime level, so guard against it.
func (inst *Instance) Stop() error {
	log.Printf("[endpoint.proxy.server] stopped")
	if inst.ln == nil {
		return nil
	}
	err := inst.ln.Close()
	inst.ln = nil
	return err
}
