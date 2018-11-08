package main

/* vim: set ts=2 sw=2 sts=2 ff=unix ft=go noet: */

import (
	//	"fmt"
	"io"
	"net"
	"sync"
	//	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/pkg/errors"
	"github.com/skillcoder/go-chat/proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const tokenHeaderName = "x-auth-token" // nolint: gosec

// Server - Chat server struct
type Server struct {
	Host      string
	Broadcast chan proto_chat_v1.StreamResponse

	clientMutex  sync.RWMutex
	ClientNames  map[string]int
	ClientTokens map[string]string

	streamsMutex  sync.RWMutex
	ClientStreams map[string]chan proto_chat_v1.StreamResponse
}

// NewServer - Create chat server object
func NewServer(host string) *Server {
	return &Server{
		Host: host,

		Broadcast: make(chan proto_chat_v1.StreamResponse, 128),

		ClientNames:   make(map[string]int),
		ClientTokens:  make(map[string]string),
		ClientStreams: make(map[string]chan proto_chat_v1.StreamResponse),
	}
}

// Run - main server function
func (s *Server) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ServerLogf("listening on %s", s.Host)

	srv := grpc.NewServer()
	proto_chat_v1.RegisterChatServer(srv, s)

	l, err := net.Listen("tcp", s.Host)
	if err != nil {
		return errors.WithMessage(err,
			"server unable to bind on provided host")
	}

	go s.broadcast()

	go func() {
		err := srv.Serve(l)
		if err != nil {
			ErrorLogf("error while serve request: %s", err.Error())
		}
		cancel()
	}()

	<-ctx.Done()
	ServerLogf("shutdown...")

	s.Broadcast <- s.shutdownEvent()

	close(s.Broadcast)

	srv.GracefulStop()
	return nil
}

// Login - process Login request from client
func (s *Server) Login(ctx context.Context, req *proto_chat_v1.LoginRequest) (*proto_chat_v1.LoginResponse, error) {
	if req.Username == "" {
		return nil, status.Error(codes.InvalidArgument, "username is required")
	}

	token, err := s.createToken()
	if err != nil {
		return nil, status.Error(codes.Internal, "cant create token")
	}

	s.addToken(token, req.Username)

	return &proto_chat_v1.LoginResponse{Token: token}, nil
}

// MessageStream - process stream messages from client
func (s *Server) MessageStream(srv proto_chat_v1.Chat_MessageStreamServer) error {
	token, ok := s.extractToken(srv.Context())
	if !ok {
		return status.Error(codes.Unauthenticated, "missing auth header")
	}
	// check token
	username, ok := s.getClient(token)
	if !ok {
		return status.Error(codes.Unauthenticated, "invalid token")
	}

	stream := s.subscribe(token)
	go s.sendBroadcasts(srv, stream, token)

	// RECEIVE LOOP
	for {
		req, err := srv.Recv()
		// client streaming over
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		s.Broadcast <- s.messageEvent(username, req.Message, req.Id)
	}

	<-srv.Context().Done()
	return srv.Context().Err()
}

func (s *Server) clientOnline(token string) bool {
	username, ok := s.getClient(token)
	if !ok {
		return false
	}

	if s.addClient(username) {
		ServerLogf("%s (%s) duplicate client", token, username)
	} else {
		s.Broadcast <- s.loginEvent(username)
		ServerLogf("%s (%s) has logged in", token, username)
	}

	return true
}

func (s *Server) clientOffline(token string) {
	username, ok := s.delClient(token)
	if !ok {
		return
	}

	if ok := s.isClientOnline(username); !ok {
		s.Broadcast <- s.logoutEvent(username)
	}
}

func (s *Server) loginEvent(username string) proto_chat_v1.StreamResponse {
	return proto_chat_v1.StreamResponse{
		Timestamp: ptypes.TimestampNow(),
		Event: &proto_chat_v1.StreamResponse_ClientOnline{
			ClientOnline: &proto_chat_v1.StreamResponse_Online{
				Username: username,
			},
		},
	}
}

func (s *Server) logoutEvent(username string) proto_chat_v1.StreamResponse {
	return proto_chat_v1.StreamResponse{
		Timestamp: ptypes.TimestampNow(),
		Event: &proto_chat_v1.StreamResponse_ClientOffline{
			ClientOffline: &proto_chat_v1.StreamResponse_Offline{
				Username: username,
			},
		},
	}
}

func (s *Server) messageEvent(username, message, id string) proto_chat_v1.StreamResponse {
	return proto_chat_v1.StreamResponse{
		Timestamp: ptypes.TimestampNow(),
		Event: &proto_chat_v1.StreamResponse_ClientMessage{
			ClientMessage: &proto_chat_v1.StreamResponse_Message{
				Username: username,
				Message:  message,
				Id:       id,
			},
		},
	}
}

func (s *Server) shutdownEvent() proto_chat_v1.StreamResponse {
	return proto_chat_v1.StreamResponse{
		Timestamp: ptypes.TimestampNow(),
		Event: &proto_chat_v1.StreamResponse_ServerShutdown{
			ServerShutdown: &proto_chat_v1.StreamResponse_Shutdown{},
		},
	}
}

func (s *Server) createToken() (token string, err error) {
	token, err = GenerateRandomString(32)
	if err != nil {
		// TODO create alert here with internal details for admin team
		return "", err
	}

	return token, nil
}

func (s *Server) extractToken(ctx context.Context) (token string, ok bool) {
	md, ok := metadata.FromIncomingContext(ctx)
	DebugLogf("%v", md)
	if !ok || len(md[tokenHeaderName]) == 0 {
		return "", false
	}
	return md[tokenHeaderName][0], true
}

func (s *Server) getClient(token string) (username string, ok bool) {
	s.clientMutex.RLock()
	username, ok = s.ClientTokens[token]
	s.clientMutex.RUnlock()
	return username, ok
}

func (s *Server) addToken(token string, username string) {
	s.clientMutex.Lock()
	s.ClientTokens[token] = username
	s.clientMutex.Unlock()
}

func (s *Server) addClient(username string) bool {
	isOnline := s.isClientOnline(username)

	s.clientMutex.Lock()
	s.ClientNames[username]++
	s.clientMutex.Unlock()

	return isOnline
}

func (s *Server) delClient(token string) (string, bool) {
	if username, ok := s.getClient(token); ok {
		s.clientMutex.Lock()
		delete(s.ClientTokens, token)
		if count, exist := s.ClientNames[username]; exist {
			if count <= 1 {
				delete(s.ClientNames, username)
			} else {
				s.ClientNames[username]--
			}
		} else {
			// TODO Make alarm to dev team
			WarnLogf("unexpected undefined s.ClientNames for user %s (%s)", token, username)
		}

		s.clientMutex.Unlock()

		return username, ok
	}

	return "", false
}

func (s *Server) isClientOnline(username string) (ok bool) {
	s.clientMutex.RLock()
	count, ok := s.ClientNames[username]
	s.clientMutex.RUnlock()
	if ok && count > 0 {
		return true
	}

	return false
}

func (s *Server) sendBroadcasts(
	srv proto_chat_v1.Chat_MessageStreamServer,
	stream chan proto_chat_v1.StreamResponse,
	token string,
) {
	defer s.unsubscribe(token)
	for {
		select {
		case <-srv.Context().Done():
			// client is done streaming
			return
		case res := <-stream:
			if s, ok := status.FromError(srv.Send(&res)); ok {
				switch s.Code() {
				case codes.OK:
					// noop
				default:
					// couldn’t send broadcast
					return
				}
			}
		}
	}
}

func (s *Server) broadcast() {
	for res := range s.Broadcast {
		s.streamsMutex.RLock()
		for _, stream := range s.ClientStreams {
			select {
			case stream <- res:
				// noop
			default:
				// TODO make alert for admin team
				ServerLogf("WARN: client stream is full, drop message")
			}
		}
		s.streamsMutex.RUnlock()
	}
}

func (s *Server) subscribe(token string) (stream chan proto_chat_v1.StreamResponse) {
	DebugLogf("[%s] subscribe...", token)
	stream = make(chan proto_chat_v1.StreamResponse, 100)

	s.streamsMutex.Lock()
	s.ClientStreams[token] = stream
	s.streamsMutex.Unlock()

	if ok := s.clientOnline(token); !ok {
		WarnLogf("cant online online %s", token)
	}

	DebugLogf("client subscribed for broadcasts: %s", token)
	return stream
}

func (s *Server) unsubscribe(token string) {
	DebugLogf("[%s] unsubscribe...", token)
	s.streamsMutex.Lock()

	if stream, ok := s.ClientStreams[token]; ok {
		delete(s.ClientStreams, token)
		close(stream)
	}

	s.streamsMutex.Unlock()

	s.clientOffline(token)
	DebugLogf("unsubscribed from broadcasts: %s", token)
}
