// Native host handles one sendNativeMessage invocation. Stdout is framed JSON only.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/ipc"
	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/protocol"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), protocol.Timeout)
	defer cancel()
	type result struct {
		m protocol.Message
		e error
	}
	ch := make(chan result, 1)
	go func() { m, e := protocol.Read(os.Stdin); ch <- result{m, e} }()
	var request protocol.Message
	select {
	case r := <-ch:
		if r.e != nil {
			fmt.Fprintln(os.Stderr, "Invalid native message:", r.e)
			os.Exit(1)
		}
		request = r.m
	case <-ctx.Done():
		fmt.Fprintln(os.Stderr, "Native message timed out after five seconds")
		os.Exit(1)
	}
	reply, e := ipc.Exchange(ctx, request)
	if e != nil {
		reply = protocol.Message{Version: protocol.Version, ID: request.ID, Type: "error", Error: "Open IDM Enhanced and retry. Live browser IPC currently requires Linux."}
		fmt.Fprintln(os.Stderr, "Desktop connection failed:", e)
	}
	if e = protocol.Write(os.Stdout, reply); e != nil {
		fmt.Fprintln(os.Stderr, "Write native response:", e)
		os.Exit(1)
	}
}
