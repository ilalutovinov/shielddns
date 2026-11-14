# ShieldDNS — облачный DNS с блокировкой рекламы (MVP)

Аналог NextDNS / AdGuard DNS: персональный адрес `a7f9k2.dns.<домен>`, DoH (RFC 8484), DoT (853), обычный DNS (53),
личный кабинет со статистикой, фильтрами, мастером настройки и генерацией `.mobileconfig` для iOS/macOS.

## Архитектура

```
 устройство ──DoH──► Caddy :443 ──► dns:8053 ─┐
            ──DoT──► dns:853 (SNI → client_id)├─► Resolver ─► whitelist → blacklist → категории → апстрим (+кэш Redis)
            ──DNS──► dns:53 (IP → профиль)   ─┘        │
                                                       └─► канал событий ─► воркер (раз в 4 с) ─► Postgres (батч) + лента в Redis
 браузер ──► Caddy ──► frontend (Next.js) / api (Go, REST + JWT) ──► Postgres, Redis
```

* **Профиль** (`client_id`) берётся из `Host` (DoH), SNI (DoT) или пути `/dns-query/<id>` (фолбэк DoH). Для порта 53 — по IP (кнопка
  «Привязать мой IP» в кабинете), `DEFAULT_CLIENT_ID` или встроенный профиль по умолчанию (`ALLOW_ANON_DEFAULT`).
* **Профили**: память (15 с) → Redis (5 мин) → Postgres. API при изменениях сбрасывает кэш через Redis pub/sub.
* **Блоклисты**: скачиваются фоном (StevenBlack, OISD, URLhaus, соцсети, 18+), хранятся как `map[string]struct{}`. Проверка домена и всех его
  родителей — O(число меток). Новая версия подменяется атомарно, при ошибке загрузки остаётся прежняя.
* **Статистика**: DNS-путь только кладёт событие в буферизованный канал (при переполнении событие отбрасывается, DNS не тормозит).
  Воркер пишет пачками (`unnest` → `INSERT`), обновляет часовые агрегаты, топ доменов, месячные счётчики и «живую ленту» в Redis. Логи старше 30 дней удаляются.
* **Лимит Free**: 50 000 запросов в месяц (счётчик в Redis, при превышении — `REFUSED`), до 10 кастомных правил.
* **Оплата — заглушка**: переключение тарифа в кабинете сразу активирует Pro на 30 дней.

## Быстрый старт локально

Нужны Docker; для режима разработки ещё Go 1.23+ и Node 20+.

```bash
cp .env.example .env            # поменяйте JWT_SECRET и пароль БД
```

### Вариант A — всё в Docker
```bash
docker compose --profile full up -d --build
```
Откройте https://localhost (Caddy выпустит локальный сертификат; браузер может попросить доверять внутреннему CA).
Регистрация → в разделе «Обзор» вы увидите адреса вида `a7f9k2xq.dns.localhost`.

### Вариант B — разработка (Go и фронт на хосте)
```bash
make db            # Postgres + Redis (схема применяется автоматически)
make dev-dns       # DNS: UDP/TCP :5353, DoT :8853, DoH :8053
make dev-api       # API :8080            (в другом терминале)
make dev-front     # сайт :3000 → /api проксируется на :8080
```
Если схема менялась, пересоздайте БД: `docker compose down -v && make db`.

## Проверка работы

Блоклисты подгружаются в первые секунды после старта (смотрите лог `blocklists: ads загружен…`).

**Обычный DNS** (профиль по умолчанию, `ALLOW_ANON_DEFAULT=true`):
```bash
dig @127.0.0.1 -p 5353 doubleclick.net        # → 0.0.0.0 (заблокирован)
dig @127.0.0.1 -p 5353 wikipedia.org          # → настоящий IP
```

**DoH** (ID возьмите из кабинета; `Host` определяет профиль, путь — фолбэк):
```bash
ID=a7f9k2xq
# GET по RFC 8484 (запрос «A doubleclick.net», base64url)
curl -s -H "Host: $ID.dns.localhost" -H 'accept: application/dns-message' \
  "http://127.0.0.1:8053/dns-query?dns=$(python3 - <<'PY'
import base64,dns.message
print(base64.urlsafe_b64encode(dns.message.make_query('doubleclick.net','A').to_wire()).rstrip(b'=').decode())
PY
)" | python3 -c "import sys,dns.message;print(dns.message.from_wire(sys.stdin.buffer.read()))"

# Фолбэк-путь /dns-query/<id> и POST через doh-клиент (q, doggo, kdig):
doggo doubleclick.net A --type A @https://localhost/dns-query/$ID --skip-hostname-verification
kdig +https +tls-hostname=$ID.dns.localhost @127.0.0.1 doubleclick.net   # если установлен knot-dnsutils
```
(`pip install dnspython` для примера выше.) Через Caddy: `curl -k --resolve $ID.dns.localhost:443:127.0.0.1 "https://$ID.dns.localhost/dns-query?dns=..."`.

**DoT** (SNI определяет профиль; в dev сертификат самоподписанный, поэтому `+tls-ca` не проверяем):
```bash
kdig -d @127.0.0.1 -p 8853 +tls +tls-hostname=$ID.dns.localhost doubleclick.net      # локальный запуск
kdig @127.0.0.1 -p 853 +tls +tls-hostname=$ID.dns.localhost doubleclick.net          # docker full
```
Для проверки сертификата в проде используйте `+tls-ca +tls-hostname=...` с системным хранилищем.

## Тесты
```bash
make test          # go mod tidy && go test ./...
```
Покрыто: порядок whitelist → blacklist → категории, субдомены, A/AAAA/NXDOMAIN, кэш и подмена ID ответа, лимит Free, отказ неизвестному профилю,
парсер блоклистов, извлечение `client_id` из Host/SNI, генерация `.mobileconfig`.

## Развёртывание на VPS с wildcard SSL

1. **DNS-записи** у регистратора/Cloudflare: `A example.com → IP VPS` и `A *.dns.example.com → IP VPS` (серым облаком, без прокси!).
2. **Порты** на VPS: 80, 443 (сайт и DoH), **853** (DoT), **53** udp/tcp (если нужен обычный DNS). Освободите порт 53:
   `sudo systemctl disable --now systemd-resolved` (и пропишите `nameserver 1.1.1.1` в `/etc/resolv.conf`).
3. **.env**:
   ```
   BASE_DOMAIN=example.com
   ACME_EMAIL=you@example.com
   CADDYFILE=./deploy/caddy/Caddyfile.prod
   CLOUDFLARE_API_TOKEN=...        # токен с правом Zone:DNS:Edit
   DNS_PUBLIC_PORT=53
   JWT_SECRET=$(openssl rand -hex 32)
   POSTGRES_PASSWORD=<длинный пароль>
   ALLOW_ANON_DEFAULT=false        # на публичном сервере не отдавать открытый резолвер всем
   ```
4. `docker compose --profile full up -d --build`
5. **Wildcard-сертификат**: Caddy (собран с плагином `caddy-dns/cloudflare`) получает `*.dns.example.com` через ACME DNS-01.
   Для другого провайдера замените плагин в `deploy/caddy/Dockerfile` и блок `tls { dns ... }` в `Caddyfile.prod`.
6. **DoT использует тот же сертификат**: контейнер `dns` читает его из тома Caddy
   (`.../wildcard_.dns.example.com/*.crt|key`) и подхватывает продление без перезапуска. Проверьте лог `dns`:
   нет сообщения «использую самоподписанный». Если путь у вас отличается — задайте `DOT_CERT`/`DOT_KEY` вручную.
7. Проверка: `kdig @IP +tls +tls-hostname=$ID.dns.example.com doubleclick.net`, `curl https://$ID.dns.example.com/dns-query?...`.

### Безопасность в проде
* Обычный DNS на :53 без привязки — открытый резолвер (угроза DNS-amplification): держите `ALLOW_ANON_DEFAULT=false`, ограничьте порт файрволом.
* Порты Postgres/Redis уже слушают только `127.0.0.1`. API/DoH наружу идут только через Caddy.
* Вход ограничен 10 попытками в минуту с IP; пароли — bcrypt; JWT живёт 7 дней. Для продакшена добавьте почтовое подтверждение и сброс пароля.
* `.mobileconfig` не подписан — iOS покажет «Не подписан». Подпись требует сертификата разработчика.

## Структура
```
backend/cmd/{dns,api}          точки входа
backend/internal/dnscore       резолвер, DoH/DoT/UDP, TLS
backend/internal/blocklist     парсер и хранилище списков
backend/internal/store         профили (память→Redis→PG), кэш, лимиты
backend/internal/stats         асинхронный сбор статистики
backend/internal/api           REST: auth, профиль, фильтры, правила, статистика, подписка, mobileconfig
backend/migrations             схема БД
frontend/                      Next.js 14 + Tailwind + Recharts
deploy/caddy                   Caddyfile (dev/prod) и сборка Caddy с DNS-плагином
```

## REST API (кратко)
`POST /api/auth/{register,login}` · `GET /api/me` · `PATCH /api/profile` · `GET|PUT /api/profile/filters` ·
`GET|POST /api/profile/rules`, `DELETE /api/profile/rules/{id}` · `GET /api/stats?range=24h|7d|30d` · `GET /api/logs` ·
`GET /api/profile/mobileconfig?proto=https|tls` · `POST /api/profile/link-ip` · `POST /api/subscription {"plan":"free|pro"}`
