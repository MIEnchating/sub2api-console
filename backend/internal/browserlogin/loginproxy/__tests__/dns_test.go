package loginproxy_test

import (
	"context"
	"net"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin/loginproxy"
	"golang.org/x/net/dns/dnsmessage"
)

func TestProxyDNSRejectsAHostWithMixedPublicAndPrivateAnswers(t *testing.T) {
	for _, privateAnswer := range []bool{false, true} {
		name := "public-only"
		if privateAnswer {
			name = "mixed-private"
		}
		t.Run(name, func(t *testing.T) {
			socket, err := net.ListenPacket("udp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan struct{})
			go func() {
				defer close(done)
				buffer := make([]byte, 4096)
				for {
					size, peer, err := socket.ReadFrom(buffer)
					if err != nil {
						return
					}
					var message dnsmessage.Message
					if message.Unpack(buffer[:size]) != nil {
						continue
					}
					message.Header.Response = true
					message.Header.RecursionAvailable = true
					for _, question := range message.Questions {
						if question.Type != dnsmessage.TypeA {
							continue
						}
						header := dnsmessage.ResourceHeader{Name: question.Name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET, TTL: 60}
						message.Answers = append(message.Answers, dnsmessage.Resource{Header: header, Body: &dnsmessage.AResource{A: [4]byte{8, 8, 8, 8}}})
						if privateAnswer {
							message.Answers = append(message.Answers, dnsmessage.Resource{Header: header, Body: &dnsmessage.AResource{A: [4]byte{127, 0, 0, 1}}})
						}
					}
					response, err := message.Pack()
					if err == nil {
						_, _ = socket.WriteTo(response, peer)
					}
				}
			}()
			previous := net.DefaultResolver
			net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "udp", socket.LocalAddr().String())
			}}
			t.Cleanup(func() { net.DefaultResolver = previous; _ = socket.Close(); <-done })
			address, err := loginproxy.PublicAddress(context.Background(), "proxy.example")
			if privateAnswer {
				if err == nil || address != "" {
					t.Fatal("a mixed DNS result bypassed the public-address policy")
				}
			} else if err != nil || address != "8.8.8.8" {
				t.Fatalf("public DNS result = %q, %v", address, err)
			}
		})
	}
}
