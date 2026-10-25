package mailoutbox

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

// Exercise the same SMTP client construction as Send against a relay that
// rejects the default localhost greeting before advertising STARTTLS.
func TestSMTPSenderDomainGreetingPrecedesTLS(t *testing.T) {
	for _, from := range []string{"accounts@example.test", "security@example.test"} {
		t.Run(from, func(t *testing.T) {
			clientSide, relay := net.Pipe()
			defer clientSide.Close()
			defer relay.Close()
			_ = clientSide.SetDeadline(time.Now().Add(2 * time.Second))
			_ = relay.SetDeadline(time.Now().Add(2 * time.Second))
			greeting := make(chan string, 1)
			finished := make(chan error, 1)
			go func() {
				if _, err := fmt.Fprint(relay, "220 relay.example.test ready\r\n"); err != nil {
					finished <- err
					return
				}
				line, err := bufio.NewReader(relay).ReadString('\n')
				if err != nil {
					finished <- err
					return
				}
				greeting <- strings.TrimSpace(line)
				if line != "EHLO example.test\r\n" {
					_, err = fmt.Fprint(relay, "421 4.7.0 Try again later, closing connection\r\n")
				} else {
					_, err = fmt.Fprint(relay, "250-relay.example.test\r\n250 STARTTLS\r\n")
				}
				finished <- err
			}()
			config := SMTPConfig{Host: "relay.example.test", Port: 587, TLS: "starttls", From: from}
			if err := config.Validate(); err != nil {
				t.Fatal(err)
			}
			client, err := config.newClient(clientSide)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			if ok, _ := client.Extension("STARTTLS"); !ok {
				t.Fatal("sender greeting did not preserve STARTTLS negotiation")
			}
			if got := <-greeting; got != "EHLO example.test" {
				t.Fatalf("unexpected greeting %q", got)
			}
			if err := <-finished; err != nil {
				t.Fatal(err)
			}
		})
	}
}
