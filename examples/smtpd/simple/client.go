package main

import (
	"log"
	"net/smtp"
)

func main() {
	//a := smtp.PlainAuth("", "user@example.com", "password", "mail.example.com")
	// Connect to the server, authenticate, set the sender and recipient,
	// and send the email all in one step.
	to := []string{"recipient@example.net"}
	msg := []byte("To: recipient@example.net\r\n" +
		"Subject: discount Gophers!\r\n" +
		"\r\n" +
		"This is the email body.\r\n")

	c, err := smtp.Dial(":1025")
	if err != nil {
		log.Fatalln("error: %v", err)
	}
	defer c.Close()
	if err = c.Hello("localhost"); err != nil {
		log.Fatalln("error: %v", err)
	}
	if err = c.Mail("sender@example.org"); err != nil {
		log.Fatalln("error: %v", err)
	}
	for _, addr := range to {
		if err = c.Rcpt(addr); err != nil {
			log.Fatalln("error: %v", err)
		}
	}
	w, err := c.Data()
	if err != nil {
		log.Fatalln("error: %v", err)
	}
	_, err = w.Write(msg)
	if err != nil {
		log.Fatalln("error: %v", err)
	}
	err = w.Close()
	if err != nil {
		log.Fatalln("error: %v", err)
	}
	c.Quit()
}