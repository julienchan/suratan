package main

import (
	"net"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"log"
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
				io.ReadWriteCloser(conn),
			)

			<-sem
		}()
	}
}

func Accept(remoteAddress string, conn io.ReadWriteCloser) {
	proto := smtpd.NewProtocol(conn, &DumpHandler{})
	proto.StartSession()
}

type DumpHandler struct {
}

func (h *DumpHandler) Logf(msg string, args ...interface{}) {
	log.Printf(msg, args...)
}

func (h *DumpHandler) MessageReceived(msg *smtpd.SMTPMessage) (string, error) {
	size := 32

	rb := make([]byte, size)
	_, err := rand.Read(rb)

	if err != nil {
		return "", nil
	}

	rs := base64.URLEncoding.EncodeToString(rb)

	b, err := ioutil.ReadAll(msg.Reader())
	if err != nil {
		return "", nil
	}
	fmt.Println(string(b))

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