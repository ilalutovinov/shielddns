package mobileconfig

import (
	"strings"
	"testing"
)

func TestGenerate(t *testing.T) {
	doh := string(Generate("a7f9k2", "example.com", "https"))
	if !strings.Contains(doh, "https://a7f9k2.dns.example.com/dns-query") || !strings.Contains(doh, "com.apple.dnsSettings.managed") {
		t.Fatal("DoH-профиль собран неверно")
	}
	dot := string(Generate("a7f9k2", "example.com", "tls"))
	if !strings.Contains(dot, "<string>a7f9k2.dns.example.com</string>") {
		t.Fatal("DoT-профиль собран неверно")
	}
	if string(Generate("a7f9k2", "example.com", "https")) != doh {
		t.Fatal("UUID должны быть детерминированными")
	}
}
