package smtpd

type State int

const (
	INVALID   = State(-1)
	ESTABLISH = State(iota)
	AUTHPLAIN
	AUTHLOGIN
	AUTHLOGIN2
	AUTHCRAMMD5
	MAIL
	RCPT
	DATA
	DONE
)

var StateMap = map[State]string{
	INVALID:     "INVALID",
	ESTABLISH:   "ESTABLISH",
	AUTHPLAIN:   "AUTHPLAIN",
	AUTHLOGIN:   "AUTHLOGIN",
	AUTHLOGIN2:  "AUTHLOGIN2",
	AUTHCRAMMD5: "AUTHCRAMMD5",
	MAIL:        "MAIL",
	RCPT:        "RCPT",
	DATA:        "DATA",
	DONE:        "DONE",
}