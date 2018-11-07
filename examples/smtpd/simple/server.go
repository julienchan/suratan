package main

import (
	"net"
	"crypto/rand"
	"encoding/base64"
	"io"
	"log"
	"os"
	"github.com/julienchan/suratan/smtpd"
)

func main() {
	ln, err := net.Listen("tcp", ":1025")
	if err != nil {
		log.Fatalf("[SMTP] Error listening on socket: %v\n", err)
	}
	defer ln.Close()

	sem := make(chan int, 100)

	for {
		sem <- 1

		conn, err := ln.Accept()
		if err != nil {
			log.Printf("[SMTP] Error accepting connection: %s\n", err)
			continue
		}

		go func() {
			Accept(
				conn.(*net.TCPConn).RemoteAddr().String(),
				conn,
			)

			<-sem
		}()
	}
}

func Accept(remoteAddress string, conn net.Conn) {
	proto := smtpd.NewProtocol(conn, &DumpHandler{})
	proto.StartSession()
}

type DumpHandler struct {
}

func (h *DumpHandler) MessageReceived(msg *smtpd.SMTPMessage) (string, error) {
	size := 32

	rb := make([]byte, size)
	_, err := rand.Read(rb)

	if err != nil {
		return "", nil
	}

	rs := base64.URLEncoding.EncodeToString(rb)

	dst := os.Stdout

	if _, err := dst.Write([]byte("HELO:<" + msg.Helo + ">\r\n")); err != nil {
		return "", nil
	}
	if _, err := dst.Write([]byte("FROM:<" + msg.From + ">\r\n")); err != nil {
		return "", nil
	}
	for _, t := range msg.To {
		if _, err := dst.Write([]byte("TO:<" + t + ">\r\n")); err != nil {
			return "", nil
		}
	}

	if _, err := dst.Write([]byte("\r\n")); err != nil {
		return "", err
	}

	if _, err := io.Copy(dst, msg.Data); err != nil {
		return "", err
	}

	return rs, nil
}

func (h *DumpHandler) ValidateSender(from string) bool {
	return true
}

func (h *DumpHandler) ValidateRecipient(to string) bool {
	return true
}

func (h *DumpHandler) Authenticate(mechanism string, args ...string) (errorReply *smtpd.Reply, ok bool) {
	return nil, true
}

func (h *DumpHandler) GetAuthenticationMechanisms() []string {
	return []string{"PLAIN"}
}

func (h *DumpHandler) SMTPVerbFilter(verb string, args ...string) (errorReply *smtpd.Reply) {
	return nil
}

func (h *DumpHandler) TLSHandler(done func(ok bool)) (errorReply *smtpd.Reply, callback func(), ok bool) {
	return nil, func() {
		done(false)
	}, true
}
