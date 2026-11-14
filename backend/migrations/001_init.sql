-- ShieldDNS: схема БД (PostgreSQL 16)
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS citext;

CREATE TYPE plan_tier AS ENUM ('free', 'pro');

CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         CITEXT NOT NULL UNIQUE,
    password_hash TEXT   NOT NULL,                  -- argon2id / bcrypt
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE subscriptions (
    user_id    UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    plan       plan_tier   NOT NULL DEFAULT 'free',
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ                          -- NULL = бессрочно (free). Оплата — заглушка в MVP
);

-- DNS-профиль: client_id — это поддомен a7f9k2.dns.<домен>
CREATE TABLE profiles (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    client_id     TEXT NOT NULL UNIQUE CHECK (client_id ~ '^[a-z0-9]{6,12}$'),
    name          TEXT NOT NULL DEFAULT 'Мой профиль',
    block_mode    TEXT NOT NULL DEFAULT 'null_ip' CHECK (block_mode IN ('null_ip','nxdomain')),
    logging       BOOLEAN NOT NULL DEFAULT TRUE,
    last_known_ip INET,                             -- для привязки по IP в обычном DNS :53
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_profiles_user ON profiles(user_id);
CREATE INDEX idx_profiles_ip   ON profiles(last_known_ip);

-- Глобальные категории блоклистов
CREATE TABLE blocklists (
    id          SERIAL PRIMARY KEY,
    slug        TEXT NOT NULL UNIQUE,               -- ads, malware, social, adult
    category    TEXT NOT NULL,
    title       TEXT NOT NULL,
    source_url  TEXT NOT NULL,
    format      TEXT NOT NULL DEFAULT 'hosts' CHECK (format IN ('hosts','domains','adblock')),
    enabled     BOOLEAN NOT NULL DEFAULT TRUE,
    domain_count INT NOT NULL DEFAULT 0,
    updated_at  TIMESTAMPTZ
);

-- Какие списки включены у профиля
CREATE TABLE profile_blocklists (
    profile_id UUID NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    blocklist_id INT NOT NULL REFERENCES blocklists(id) ON DELETE CASCADE,
    PRIMARY KEY (profile_id, blocklist_id)
);

-- Кастомные правила (whitelist имеет приоритет над blacklist)
CREATE TABLE custom_rules (
    id         BIGSERIAL PRIMARY KEY,
    profile_id UUID NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    domain     TEXT NOT NULL,                       -- нормализован: lower-case, без точки на конце
    action     TEXT NOT NULL CHECK (action IN ('allow','block')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (profile_id, domain)
);

-- Сырые логи (батч-вставка воркером). Партиции по дням — задел на рост
CREATE TABLE query_logs (
    id         BIGSERIAL,
    ts         TIMESTAMPTZ NOT NULL,
    profile_id UUID NOT NULL,
    domain     TEXT NOT NULL,
    qtype      TEXT NOT NULL,
    status     TEXT NOT NULL CHECK (status IN ('allowed','blocked')),
    reason     TEXT,                                -- whitelist | blacklist | list:ads | ...
    PRIMARY KEY (id, ts)
) PARTITION BY RANGE (ts);
CREATE TABLE query_logs_default PARTITION OF query_logs DEFAULT;
CREATE INDEX idx_logs_profile_ts ON query_logs (profile_id, ts DESC);

-- Агрегаты по часам для быстрых графиков
CREATE TABLE stats_hourly (
    profile_id UUID NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    hour       TIMESTAMPTZ NOT NULL,
    total      INT NOT NULL DEFAULT 0,
    blocked    INT NOT NULL DEFAULT 0,
    PRIMARY KEY (profile_id, hour)
);

-- Топ доменов за сутки
CREATE TABLE stats_domains_daily (
    profile_id UUID NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    day        DATE NOT NULL,
    domain     TEXT NOT NULL,
    status     TEXT NOT NULL,
    hits       INT NOT NULL DEFAULT 0,
    PRIMARY KEY (profile_id, day, domain, status)
);

-- Месячный счётчик для лимита Free (50 000), быстрее считать из Redis, здесь — источник истины
CREATE TABLE usage_monthly (
    profile_id UUID NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    month      DATE NOT NULL,
    queries    BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (profile_id, month)
);

INSERT INTO blocklists (slug, category, title, source_url, format) VALUES
 ('ads',     'ads',     'Реклама и трекеры (StevenBlack)', 'https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts', 'hosts'),
 ('oisd',    'ads',     'OISD Basic',                       'https://small.oisd.nl/domainswild',                                  'domains'),
 ('malware', 'malware', 'Вредоносные сайты (URLhaus)',      'https://urlhaus.abuse.ch/downloads/hostfile/',                       'hosts'),
 ('social',  'social',  'Соцсети',                          'https://raw.githubusercontent.com/StevenBlack/hosts/master/alternates/social-only/hosts', 'hosts'),
 ('adult',   'adult',   'Контент 18+',                      'https://raw.githubusercontent.com/StevenBlack/hosts/master/alternates/porn-only/hosts',   'hosts');
