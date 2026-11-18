// Package ipc transports validated ping requests to the running application.
package ipc

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/Mahmoud-s-Khedr/idm-enhanced/internal/protocol"
)

func Exchange(ctx context.Context, m protocol.Message) (protocol.Message, error) {
	if e := protocol.ValidateRequest(m); e != nil {
		return protocol.Message{}, e
	}
	ctx, cancel := context.WithTimeout(ctx, protocol.Timeout)
	defer cancel()
	conn, e := dial(ctx)
	if e != nil {
		return protocol.Message{}, fmt.Errorf("open IDM Enhanced and retry: %w", e)
	}
	defer func() { _ = conn.Close() }()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	deadline, _ := ctx.Deadline()
	if e = conn.SetDeadline(deadline); e != nil {
		return protocol.Message{}, e
	}
	if e = protocol.Write(conn, m); e != nil {
		return protocol.Message{}, e
	}
	reply, e := protocol.Read(conn)
	if e != nil {
		return reply, e
	}
	return reply, protocol.Match(m, reply)
}

// Serve limits simultaneous connections and closes all waits on cancellation.
func Serve(ctx context.Context, l net.Listener, handle func(protocol.Message) (protocol.Message, error)) error {
	stop := context.AfterFunc(ctx, func() { _ = l.Close() })
	defer stop()
	var workers sync.WaitGroup
	defer workers.Wait()
	slots := make(chan struct{}, 8)
	for {
		conn, e := l.Accept()
		if e != nil {
			if ctx.Err() != nil {
				return nil
			}
			return e
		}
		select {
		case slots <- struct{}{}:
		default:
			_ = conn.Close()
			continue
		}
		workers.Add(1)
		go func() {
			defer workers.Done()
			defer func() { <-slots }()
			defer func() { _ = conn.Close() }()
			stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
			defer stop()
			if e := conn.SetDeadline(time.Now().Add(protocol.Timeout)); e != nil {
				return
			}
			m, e := protocol.Read(conn)
			if e != nil {
				return
			}
			reply, e := handle(m)
			if e != nil {
				return
			}
			_ = protocol.Write(conn, reply)
		}()
	}
}
