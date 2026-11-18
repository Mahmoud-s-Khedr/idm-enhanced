// Native host handles one sendNativeMessage invocation. Stdout is framed JSON only.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/ipc"
	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/nativehost"
	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/protocol"
)

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "--register" || os.Args[1] == "--unregister") {
		path, err := os.Executable()
		if err == nil {
			err = nativehost.Register(path, os.Args[1] == "--unregister")
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

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
		reply = protocol.Message{Version: protocol.Version, ID: request.ID, Type: "error", Error: "Open IDM Enhanced and retry the same request. Check native host registration if the problem persists."}
		fmt.Fprintln(os.Stderr, "Desktop connection failed; open the application and retry")
	}
	if e = protocol.Write(os.Stdout, reply); e != nil {
		fmt.Fprintln(os.Stderr, "Write native response:", e)
		os.Exit(1)
	}
}
