package main

/* vim: set ts=2 sw=2 sts=2 ff=unix ft=go noet: */

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/pkg/errors"
	"github.com/skillcoder/go-chat/proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const tokenHeaderName = "x-auth-token" // nolint: gosec

type tAtomBool struct{ flag int32 }

func (b *tAtomBool) Set(value bool) {
	var i int32
	if value {
		i = 1
	}

	atomic.StoreInt32(&(b.flag), i)
}

func (b *tAtomBool) Get() bool {
	return atomic.LoadInt32(&(b.flag)) != 0
}

// Client - Chat client struct
type Client struct {
	proto_chat_v1.ChatClient
	Host     string
	Username string
	Token    string
	Shutdown *tAtomBool
	Text     string

	messageMutex sync.RWMutex
	messages     map[string]string
}

// NewClient - Create chat client object
func NewClient(host, username string) *Client {
	return &Client{
		Host:     host,
		Username: username,
		Shutdown: &tAtomBool{0},
		messages: make(map[string]string),
	}
}

// Run - main client function
func (c *Client) Run(ctx context.Context) error {
	connCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	DebugLogf("connecting...")
	conn, err := grpc.DialContext(connCtx, c.Host, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return errors.WithMessage(err, "unable to connect")
	}
	defer func() {
		e := conn.Close()
		if e != nil {
			ErrorLogf("errro while close connection: %v", e)
		}
	}()

	c.ChatClient = proto_chat_v1.NewChatClient(conn)

	DebugLogf("login...")
	if c.Token, err = c.login(ctx); err != nil {
		return errors.WithMessage(err, "failed to login")
	}
	DebugLogf("login successfully complited")

	err = c.stream(ctx)

	return errors.WithMessage(err, "stream error")
}

func (c *Client) login(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 1000*time.Millisecond)
	defer cancel()

	res, err := c.ChatClient.Login(ctx, &proto_chat_v1.LoginRequest{
		Username: c.Username,
	})

	if err != nil {
		return "", err
	}

	return res.Token, nil
}

func (c *Client) stream(ctx context.Context) error {
	md := metadata.New(map[string]string{tokenHeaderName: c.Token})
	ctx = metadata.NewOutgoingContext(ctx, md)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	client, err := c.ChatClient.MessageStream(ctx)
	if err != nil {
		DebugLogf("unknow error in c.ChatClient.MessageStream: %v", err)
		return err
	}
	defer func() {
		err := client.CloseSend()
		if err != nil {
			ErrorLogf("errro while close stream: %v", err)
		}
	}()

	DebugLogf("connected to stream")

	go c.send(client)
	return c.receive(client)
}

func (c *Client) send(client proto_chat_v1.Chat_MessageStreamClient) {
	c.showPrompt()
	sl := bufio.NewScanner(os.Stdin)
	sl.Split(bufio.ScanLines)

	for {
		select {
		case <-client.Context().Done():
			DebugLogf("client disconnected")
		default:
			if sl.Scan() {
				var msg = sl.Text()
				DebugLogf("Send: %s", msg)
				id := c.addMessage(msg)
				if err := client.Send(&proto_chat_v1.StreamRequest{Message: msg, Id: id}); err != nil {
					ErrorLogf("failed to send message: %v", err)
					return
				}
			} else {
				if sl.Err() == nil {
					DebugLogf("Batch mode, so exit")
					c.Shutdown.Set(true)
					return
				}

				ErrorLogf("input scanner failure: %v", sl.Err())
				return
			}
		}
	}
}

func (c *Client) receive(client proto_chat_v1.Chat_MessageStreamClient) error {
	for {
		res, err := client.Recv()
		s, ok := status.FromError(err)
		if ok && s.Code() == codes.Canceled {
			DebugLogf("stream canceled (usually indicates shutdown)")
			return nil
		} else if ok && s.Code() == codes.Unavailable {
			DebugLogf("stream unavailable (usually indicates network problem or server crash)")
			return nil
		} else if err == io.EOF {
			DebugLogf("stream closed by server")
			return nil
		} else if err != nil {
			DebugLogf("unknow error in receive: %v", err)
			return err
		}

		switch evt := res.Event.(type) {
		case *proto_chat_v1.StreamResponse_ClientOnline:
			c.printStatusMessage(evt.ClientOnline.Username, "online")
			c.showPrompt()
		case *proto_chat_v1.StreamResponse_ClientOffline:
			c.printStatusMessage(evt.ClientOffline.Username, "offline")
			c.showPrompt()
		case *proto_chat_v1.StreamResponse_ClientMessage:
			if c.checkMessage(evt.ClientMessage.Id) {
				c.delMessage(evt.ClientMessage.Id)
			} else {
				c.printMessage(evt.ClientMessage.Username, evt.ClientMessage.Message)
			}
			if c.Shutdown.Get() && evt.ClientMessage.Username == c.Username {
				if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
					ErrorLogf("cant terminate our client [pid=%d]: %v", os.Getpid(), err)
					return nil
				}
			}

			c.showPrompt()
		case *proto_chat_v1.StreamResponse_ServerShutdown:
			DebugLogf("server is shutting down")
			c.Shutdown.Set(true)
			if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
				ErrorLogf("cant terminate our client [pid=%d]: %v", os.Getpid(), err)
				return nil
			}
		default:
			WarnLogf("unexpected event from the server: %T", evt)
			return nil
		}
	}
}

func (c *Client) printMessage(username, text string) {
	fmt.Printf("\033[1K\r%s: %s\n", username, text)
}

func (c *Client) printStatusMessage(username, state string) {
	fmt.Printf("\033[1K\rserver: %s is %s\n", username, state)
}

func (c *Client) addMessage(message string) string {
	n := time.Now().UnixNano()
	id := strconv.FormatInt(n, 16)

	c.messageMutex.Lock()
	c.messages[id] = message
	c.messageMutex.Unlock()

	return id
}

func (c *Client) checkMessage(id string) bool {
	c.messageMutex.RLock()
	_, ok := c.messages[id]
	c.messageMutex.RUnlock()
	return ok
}

func (c *Client) delMessage(id string) {
	c.messageMutex.Lock()
	delete(c.messages, id)
	c.messageMutex.Unlock()
}

func (c *Client) showPrompt() {
	fmt.Printf("%s: %s", c.Username, c.Text)
}
