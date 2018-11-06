package smtpd

import (
	"io"
	"log"
	"net/textproto"
	"strings"
)

type Command struct {
	cmd string
	args string
	orig string
}

type SMTPMessage struct {
	From string
	To   []string
	Data string
	Helo string
}

func ParseCommand(line string) *Command {
	words := strings.Split(line, " ")
	cmd := strings.ToUpper(words[0])
	args := strings.Join(words[1:len(words)], " ")

	return &Command{
		cmd: command,
		args: args,
		orig: line,
	}
}

type ProtocolHandler interface {
	Logf(msg string, args ...interface{})
	MessageReceived(*SMTPMessage) (string, error)
	ValidateSender(from string) bool
	ValidateRecipient(to string) bool
	Authenticate(mechanism string, args ...string) (errorReply *Reply, ok bool)
	GetAuthenticationMechanisms() string[]
	SMTPVerbFilter(verb string, args ...string) (errorReply *Reply)
	TLSHandler(done func(ok bool)) (errorReply *Reply, callback func(), ok bool)
}

type Protocol struct {
	lastCommand *Command
	tconn *textproto.Conn
	conn io.ReadWriteCloser

	TLSPending  bool
	TLSUpgraded bool

	State   State
	Message *SMTPMessage

	Hostname string
	Ident    string

	MaximumLineLength int
	MaximumRecipients int

	handler *ProtocolHandler

	RejectBrokenRCPTSyntax bool
	RejectBrokenMAILSyntax bool
	RequireTLS bool
}

func NewProtocol(conn io.ReadWriteCloser, handler *ProtocolHandler) *Protocol {
	p := &Protocol{
		Hostname:          "suratan.example",
		Ident:             "ESMTP Suratan",
		State:             INVALID,
		MaximumLineLength: -1,
		MaximumRecipients: -1,
		handler: handler,
		tconn: textproto.NewConn(conn),
		conn:   conn,
	}
	p.resetState()
	return p
}

func (p *Protocol) Close() error {
	return p.conn.Close()
}

func (p *Protocol) resetState() {
	p.Message = &SMTPMessage{}
}

func (p *Protocol) logf(msg string, args ...interface{}) {
	msg = strings.Join([]string{"[PROTO: %s]", msg}, " ")
	args = append([]interface{}{StateMap[p.State]}, args...)

	if p.handler != nil {
		p.handler.Logf(msg, args...)
	} else {
		log.Printf(msg, args...)
	}
}

func (p *Protocol) Start() *Reply {
	p.logf("Started session, switching to ESTABLISH state")
	p.State = ESTABLISH
	return &Reply{220, []string{p.Hostname + " " + p.Ident}}
}

func (p *Protocol) ReadReply() (*Reply, error) {
	if p.State == DATA {
		lines, err := p.tconn.Reader.ReadDotLines()
		if err != nil {
			return nil, err
		}
	}
}