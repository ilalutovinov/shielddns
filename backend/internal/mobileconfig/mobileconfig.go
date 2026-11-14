// Package mobileconfig генерирует Apple .mobileconfig с зашифрованным DNS (iOS 14+/macOS 11+).
package mobileconfig

import (
	"crypto/sha256"
	"fmt"
	"html"
)

// uuid детерминирован (по client_id), поэтому повторная установка обновляет профиль, а не дублирует его.
func uuid(seed string) string {
	h := sha256.Sum256([]byte("shielddns:" + seed))
	b := h[:16]
	b[6] = (b[6] & 0x0f) | 0x50
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%X-%X-%X-%X-%X", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// Generate: proto = "https" (DoH) или "tls" (DoT).
func Generate(clientID, baseDomain, proto string) []byte {
	host := clientID + ".dns." + baseDomain
	var settings string
	if proto == "tls" {
		settings = fmt.Sprintf(`<key>DNSProtocol</key><string>TLS</string>
			<key>ServerName</key><string>%s</string>`, html.EscapeString(host))
	} else {
		proto = "https"
		settings = fmt.Sprintf(`<key>DNSProtocol</key><string>HTTPS</string>
			<key>ServerURL</key><string>https://%s/dns-query</string>`, html.EscapeString(host))
	}
	name := "ShieldDNS (" + map[string]string{"https": "DoH", "tls": "DoT"}[proto] + ")"
	return []byte(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>PayloadContent</key>
	<array>
		<dict>
			<key>DNSSettings</key>
			<dict>
				%s
			</dict>
			<key>PayloadDescription</key><string>Шифрованный DNS ShieldDNS</string>
			<key>PayloadDisplayName</key><string>%s</string>
			<key>PayloadIdentifier</key><string>com.shielddns.%s.dns.%s</string>
			<key>PayloadType</key><string>com.apple.dnsSettings.managed</string>
			<key>PayloadUUID</key><string>%s</string>
			<key>PayloadVersion</key><integer>1</integer>
		</dict>
	</array>
	<key>PayloadDescription</key><string>Включает фильтрацию рекламы и трекеров через ShieldDNS</string>
	<key>PayloadDisplayName</key><string>%s</string>
	<key>PayloadIdentifier</key><string>com.shielddns.%s.%s</string>
	<key>PayloadRemovalDisallowed</key><false/>
	<key>PayloadType</key><string>Configuration</string>
	<key>PayloadUUID</key><string>%s</string>
	<key>PayloadVersion</key><integer>1</integer>
</dict>
</plist>
`, settings, name, clientID, proto, uuid(clientID+":"+proto+":payload"), name, clientID, proto, uuid(clientID+":"+proto+":root")))
}
