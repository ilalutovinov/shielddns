# Caddy с плагином DNS-провайдера для wildcard-сертификатов.
# Другой провайдер: замените плагин (github.com/caddy-dns/<provider>) и директиву `dns` в Caddyfile.prod.
FROM caddy:2-builder AS builder
RUN xcaddy build --with github.com/caddy-dns/cloudflare

FROM caddy:2
COPY --from=builder /usr/bin/caddy /usr/bin/caddy
