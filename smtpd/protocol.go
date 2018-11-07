package smtpd

import (
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/textproto"
	"regexp"
	"strings"
	"time"
)

type Command struct {
	cmd string
	args string
	orig string
}

type SMTPMessage struct {
	From string
	To   []string
	Data io.Reader
	Helo string
}

func ParseCommand(line string) *Command {
	words := strings.Split(line, " ")
	cmd := strings.ToUpper(words[0])
	args := strings.Join(words[1:len(words)], " ")

	return &Command{
		cmd: cmd,
		args: args,
		orig: line,
	}
}

type ProtocolHandler interface {
	MessageReceived(*SMTPMessage) (string, error)
	ValidateSender(from string) bool
	ValidateRecipient(to string) bool
	Authenticate(mechanism string, args ...string) (errorReply *Reply, ok bool)
	GetAuthenticationMechanisms() []string
	SMTPVerbFilter(verb string, args ...string) (errorReply *Reply)
	TLSHandler(done func(ok bool)) (errorReply *Reply, callback func(), ok bool)
}

type Protocol struct {
	lastCommand *Command
	conn      	net.Conn
	text 		*textproto.Conn

	TLSPending  bool
	TLSUpgraded bool

	State   State
	Message *SMTPMessage

	Hostname string
	Ident    string

	MaximumLineLength int
	MaximumRecipients int
	MaxIdleSeconds    int
	MaxMessageBytes   int64

	handler ProtocolHandler

	RejectBrokenRCPTSyntax bool
	RejectBrokenMAILSyntax bool
	RequireTLS bool
}

func NewProtocol(conn net.Conn, handler ProtocolHandler) *Protocol {
	p := &Protocol{
		Hostname:          "suratan.example",
		Ident:             "ESMTP Suratan",
		State:             INVALID,
		MaximumLineLength: -1,
		MaximumRecipients: -1,
		MaxIdleSeconds: -1,
		handler: handler,
		text: textproto.NewConn(conn),
		conn: conn,
	}
	p.resetState()
	return p
}

func (p *Protocol) Close() error {
	return p.text.Close()
}

func (p *Protocol) resetState() {
	p.Message = &SMTPMessage{}
}

func (p *Protocol) StartSession() {
	p.Start()
	defer p.Close()
	for {
		if p.State == DATA {
			id, reply := p.processData()
			p.reply(id, reply)
		} else {
			id, line, err := p.ReadLine()
			if err != nil {
				return
			}
			p.reply(id, p.Command(ParseCommand(line)))
		}
	}
}

func (p *Protocol) newDataReader() io.Reader {
	if p.MaxMessageBytes > 0 {
		return io.LimitReader(p.text.DotReader(), p.MaxMessageBytes)
	}
	return p.text.DotReader()
}

func (p *Protocol) Start() error {
	defer p.text.W.Flush()

	p.State = ESTABLISH
	r := SingleReply(220, p.Hostname + " " + p.Ident)
	_, err := r.WriteTo(p.text.W)
	return err
}

func (p *Protocol) Command(cmd *Command) *Reply {
	defer func() {
		p.lastCommand = cmd
	}()

	r := p.handler.SMTPVerbFilter(cmd.cmd)
	if r != nil {
		return r
	}

	switch {
	case p.TLSPending && !p.TLSUpgraded:
		return SingleReply(221, "Bye")

	case cmd.cmd == "RSET":
		p.State = MAIL
		p.Message = &SMTPMessage{}
		return SingleReply(250, "Ok")

	case cmd.cmd == "NOOP":
		return SingleReply(250, "Ok")

	case cmd.cmd == "QUIT":
		p.State = DONE
		return SingleReply(221, "Bye")

	case p.State == ESTABLISH:
		switch cmd.cmd {
		case "HELO":
			return p.Helo(cmd.args)

		case "EHLO":
			return p.Ehlo(cmd.args)

		case "STARTTLS":
			return p.StartTLS(cmd.args)

		default:
			return SingleReply(500, "Unrecognised command")
		}

	case cmd.cmd == "STARTTLS":
		return p.StartTLS(cmd.args)

	case p.RequireTLS && !p.TLSUpgraded:
		return SingleReply(530, "Must issue a STARTTLS command first")

	case p.State == AUTHPLAIN:
		p.State = MAIL
		// TODO error handling
		val, _ := base64.StdEncoding.DecodeString(cmd.orig)
		bits := strings.Split(string(val), string(rune(0)))

		if len(bits) < 3 {
			return SingleReply(550, "Badly formed parameter")
		}

		user, pass := bits[1], bits[2]

		if reply, ok := p.handler.Authenticate("PLAIN", user, pass); !ok {
			return reply
		}
		return SingleReply(235, "Authentication successful")
	case AUTHLOGIN == p.State:
		p.State = AUTHLOGIN2
		return SingleReply(334, "UGFzc3dvcmQ6")
	case AUTHLOGIN2 == p.State:
		p.State = MAIL
		if reply, ok := p.handler.Authenticate("LOGIN", p.lastCommand.orig, cmd.orig); !ok {
			return reply
		}
		return SingleReply(235, "Authentication successful")
	case AUTHCRAMMD5 == p.State:
		p.State = MAIL
		if reply, ok := p.handler.Authenticate("CRAM-MD5", cmd.orig); !ok {
			return reply
		}
		return SingleReply(235, "Authentication successful")
	case MAIL == p.State:
		switch cmd.cmd {
		case "AUTH":
			switch {
			case strings.HasPrefix(cmd.args, "PLAIN "):
				val, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(cmd.args, "PLAIN "))
				bits := strings.Split(string(val), string(rune(0)))

				if len(bits) < 3 {
					return SingleReply(550, "Badly formed parameter")
				}

				user, pass := bits[1], bits[2]

				if reply, ok := p.handler.Authenticate("PLAIN", user, pass); !ok {
					return reply
				}
				return SingleReply(235, "Authentication successful")
			case "LOGIN" == cmd.args:
				p.State = AUTHLOGIN
				return SingleReply(334, "VXNlcm5hbWU6")
			case "PLAIN" == cmd.args:
				p.State = AUTHPLAIN
				return SingleReply(334, "")
			case "CRAM-MD5" == cmd.args:
				p.State = AUTHCRAMMD5
				return SingleReply(334, "PDQxOTI5NDIzNDEuMTI4Mjg0NzJAc291cmNlZm91ci5hbmRyZXcuY211LmVkdT4=")
			case strings.HasPrefix(cmd.args, "EXTERNAL "):
				if reply, ok := p.handler.Authenticate("EXTERNAL", strings.TrimPrefix(cmd.args, "EXTERNAL ")); !ok {
					return reply
				}
				return SingleReply(235, "Authentication successful")
			default:
				return SingleReply(504, "Unsupported authentication mechanism")
			}
		case "MAIL":
			from, err := p.ParseMAIL(cmd.args)
			if err != nil {
				return SingleReply(550, err.Error())
			}
			if !p.handler.ValidateSender(from) {
				// TODO correct sender error response
				return SingleReply(550, "Invalid sender " + from)
			}
			p.Message.From = from
			p.State = RCPT
			return SingleReply(250, "Sender " + from + " ok")
		case "HELO":
			return p.Helo(cmd.args)
		case "EHLO":
			return p.Ehlo(cmd.args)
		default:
			return SingleReply(500, "Unrecognised command")
		}
	case RCPT == p.State:
		switch cmd.cmd {
		case "RCPT":
			if p.MaximumRecipients > -1 && len(p.Message.To) >= p.MaximumRecipients {
				return SingleReply(552, "Too many recipients")
			}
			to, err := p.ParseRCPT(cmd.args)
			if err != nil {
				return SingleReply(550, err.Error())
			}
			if !p.handler.ValidateRecipient(to) {
				// TODO correct send error response
				return SingleReply(550, "Invalid recipient " + to)
			}
			p.Message.To = append(p.Message.To, to)
			p.State = RCPT
			return SingleReply(250, "Recipient " + to + " ok")
		case "HELO":
			return p.Helo(cmd.args)
		case "EHLO":
			return p.Ehlo(cmd.args)
		case "DATA":
			p.State = DATA
			return SingleReply(354, "End data with <CR><LF>.<CR><LF>")
		default:
			return SingleReply(500, "Unrecognised command")
		}
	default:
		return SingleReply(500, "Unrecognised command")
	}
}

func (p *Protocol) processData() (uint, *Reply) {
	p.Message.Data = p.newDataReader()
	p.State = MAIL

	pid := p.text.Next()
	p.text.StartRequest(pid)

	defer func() {
		p.resetState()
		p.text.EndRequest(pid)
	}()

	id, err := p.handler.MessageReceived(p.Message)
	if err != nil {
		return 0, SingleReply(452, "Unable to store message")
	}
	return pid, SingleReply(250, "Ok: queued as " + id)
}

func (p *Protocol) reply(id uint, r *Reply) error {
	p.conn.SetWriteDeadline(p.nextDeadline())

	p.text.StartResponse(id)
	defer p.text.EndResponse(id)

	_, err := r.WriteTo(p.text.W)

	if err != nil {
		return err
	}
	err = p.text.W.Flush()

	if r.Done != nil {
		r.Done()
	}

	return err
}

func (p *Protocol) Helo(args string) *Reply {
	p.State = MAIL
	p.Message.Helo = args
	return SingleReply(250, "Hello " + args)
}

func (p *Protocol) Ehlo(args string) *Reply {
	p.State = MAIL
	p.Message.Helo = args
	replyArgs := []string{"Hello " + args, "PIPELINING"}
	if !p.TLSPending && !p.TLSUpgraded {
		replyArgs = append(replyArgs, "STARTTLS")
	}
	if !p.RequireTLS || p.TLSUpgraded {
		mechanisms := p.handler.GetAuthenticationMechanisms()
		if len(mechanisms) > 0 {
			replyArgs = append(replyArgs, "AUTH "+strings.Join(mechanisms, " "))
		}
	}
	return &Reply{250, replyArgs, nil}
}

func (p *Protocol) StartTLS(args string) *Reply {
	if p.TLSUpgraded {
		return SingleReply(500, "Unrecognised command")
	}

	if len(args) > 0 {
		return SingleReply(501, "Syntax error: no parameters allowed")
	}

	r, callback, ok := p.handler.TLSHandler(func(ok bool) {
		p.TLSUpgraded = ok
		p.TLSPending = ok
		if ok {
			p.resetState()
			p.State = ESTABLISH
		}
	})

	if !ok {
		return r
	}

	p.TLSPending = true

	return &Reply{220, []string{"Ready to start TLS"}, callback}
}

func (p *Protocol) nextDeadline() time.Time {
	if p.MaxIdleSeconds > 0 {
		return time.Now().Add(time.Duration(p.MaxIdleSeconds) * time.Second)
	}

	return time.Time{}
}

func (p *Protocol) ReadLine() (uint, string, error) {
	if err := p.conn.SetReadDeadline(p.nextDeadline()); err != nil {
		return 0, "", err
	}

	id := p.text.Next()
	p.text.StartRequest(id)

	defer p.text.EndRequest(id)

	line, err := p.text.ReadLine()
	if err != nil {
		return 0, "", err
	}

	return id, line, nil
}

var parseMailBrokenRegexp = regexp.MustCompile("(?i:From):\\s*<([^>]+)>")
var parseMailRFCRegexp = regexp.MustCompile("(?i:From):<([^>]+)>")

// ParseMAIL returns the forward-path from a MAIL command argument
func (proto *Protocol) ParseMAIL(mail string) (string, error) {
	var match []string
	if proto.RejectBrokenMAILSyntax {
		match = parseMailRFCRegexp.FindStringSubmatch(mail)
	} else {
		match = parseMailBrokenRegexp.FindStringSubmatch(mail)
	}

	if len(match) != 2 {
		return "", errors.New("Invalid syntax in MAIL command")
	}
	return match[1], nil
}

var parseRcptBrokenRegexp = regexp.MustCompile("(?i:To):\\s*<([^>]+)>")
var parseRcptRFCRegexp = regexp.MustCompile("(?i:To):<([^>]+)>")

// ParseRCPT returns the return-path from a RCPT command argument
func (proto *Protocol) ParseRCPT(rcpt string) (string, error) {
	var match []string
	if proto.RejectBrokenRCPTSyntax {
		match = parseRcptRFCRegexp.FindStringSubmatch(rcpt)
	} else {
		match = parseRcptBrokenRegexp.FindStringSubmatch(rcpt)
	}
	if len(match) != 2 {
		return "", errors.New("Invalid syntax in RCPT command")
	}
	return match[1], nil
}
