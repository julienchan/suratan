package smtpd

import (
	"io"
	"strconv"
)

// represents SMTP reply
type Reply struct {
	Status int
	Lines []string
}

func (r *Reply) WriteTo(w *io.Writer) (int64, error) {
	if len(r.Lines) == 0 {
		n, err := w.Write([]bytes(strconv.Itoa(r.Status) + "\n"))
		return int64(n), err
	}

	wrote := int64(0)
	for i, line := range r.Lines {
		if i == len(r.lines)-1 {
			n, err := w.Write([]bytes(strconv.Itoa(r.Status) + " " + line + "\r\n"))
			if err != nil {
				return int64(n) + wrote, err
			} else {
				wrote += int64(n)
			}
		} else {
			n, err := w.Write([]bytes(strconv.Itoa(r.Status) + "-" + line + "\r\n"))
			if err != nil {
				return int64(n) + wrote, err
			} else {
				wrote += int64(n)
			}
		}
	}

	return wrote, err
}