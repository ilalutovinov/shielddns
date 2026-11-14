// Package stats — асинхронный сбор статистики: DNS-путь только кладёт события в канал,
// а этот воркер раз в несколько секунд пишет пачку в Postgres и обновляет ленту в Redis.
package stats

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"shielddns/internal/model"
)

type Worker struct {
	DB    *pgxpool.Pool
	R     *redis.Client
	In    <-chan model.Event
	Every time.Duration
}

type LogRow struct {
	T      time.Time `json:"t"`
	Domain string    `json:"domain"`
	Type   string    `json:"type"`
	Status string    `json:"status"`
	Reason string    `json:"reason"`
}

func (w *Worker) Run(ctx context.Context) {
	tick := time.NewTicker(w.Every)
	clean := time.NewTicker(time.Hour)
	defer tick.Stop()
	defer clean.Stop()
	batch := make([]model.Event, 0, 4096)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		c, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		if err := w.flush(c, batch); err != nil {
			log.Printf("stats: flush %d событий: %v", len(batch), err)
		}
		cancel()
		batch = batch[:0]
	}
	for {
		select {
		case <-ctx.Done():
			for {
				select {
				case e := <-w.In:
					batch = append(batch, e)
				default:
					flush()
					return
				}
			}
		case e := <-w.In:
			batch = append(batch, e)
			if len(batch) >= 5000 {
				flush()
			}
		case <-tick.C:
			flush()
		case <-clean.C:
			_, _ = w.DB.Exec(context.Background(), `DELETE FROM query_logs WHERE ts < now() - interval '30 days'`)
		}
	}
}

type hk struct {
	pid  string
	hour time.Time
}
type dk struct {
	pid, domain, status string
	day                 time.Time
}
type mk struct {
	pid   string
	month time.Time
}

func (w *Worker) flush(ctx context.Context, batch []model.Event) error {
	var (
		ts                                  []time.Time
		pids, doms, qts, sts, rsn           []string
		hourly                              = map[hk][2]int32{}
		daily                               = map[dk]int32{}
		monthly                             = map[mk]int32{}
		feed                                = map[string][]string{}
	)
	for _, e := range batch {
		t := e.TS.UTC()
		h := hourly[hk{e.ProfileID, t.Truncate(time.Hour)}]
		h[0]++
		if e.Status == "blocked" {
			h[1]++
		}
		hourly[hk{e.ProfileID, t.Truncate(time.Hour)}] = h
		monthly[mk{e.ProfileID, time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)}]++
		if !e.Logging {
			continue
		}
		ts, pids, doms, qts, sts, rsn = append(ts, t), append(pids, e.ProfileID), append(doms, e.Domain),
			append(qts, e.QType), append(sts, e.Status), append(rsn, e.Reason)
		daily[dk{e.ProfileID, e.Domain, e.Status, time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)}]++
		b, _ := json.Marshal(LogRow{t, e.Domain, e.QType, e.Status, e.Reason})
		feed[e.ProfileID] = append(feed[e.ProfileID], string(b))
	}

	if len(ts) > 0 {
		if _, err := w.DB.Exec(ctx, `
INSERT INTO query_logs (ts, profile_id, domain, qtype, status, reason)
SELECT ts, pid::uuid, domain, qtype, status, reason
FROM unnest($1::timestamptz[], $2::text[], $3::text[], $4::text[], $5::text[], $6::text[]) AS t(ts, pid, domain, qtype, status, reason)`,
			ts, pids, doms, qts, sts, rsn); err != nil {
			return err
		}
	}

	var hp, hd, mp, dp, dd, ds []string
	var hh, mm, dday []time.Time
	var ht, hb, mq, dh []int32
	for k, v := range hourly {
		hp, hh, ht, hb = append(hp, k.pid), append(hh, k.hour), append(ht, v[0]), append(hb, v[1])
	}
	for k, v := range monthly {
		mp, mm, mq = append(mp, k.pid), append(mm, k.month), append(mq, v)
	}
	for k, v := range daily {
		dp, dd, ds, dday, dh = append(dp, k.pid), append(dd, k.domain), append(ds, k.status), append(dday, k.day), append(dh, v)
	}
	_ = hd
	if _, err := w.DB.Exec(ctx, `
INSERT INTO stats_hourly (profile_id, hour, total, blocked)
SELECT pid::uuid, hour, total, blocked FROM unnest($1::text[], $2::timestamptz[], $3::int[], $4::int[]) AS t(pid, hour, total, blocked)
ON CONFLICT (profile_id, hour) DO UPDATE SET total = stats_hourly.total + EXCLUDED.total, blocked = stats_hourly.blocked + EXCLUDED.blocked`,
		hp, hh, ht, hb); err != nil {
		return err
	}
	if _, err := w.DB.Exec(ctx, `
INSERT INTO usage_monthly (profile_id, month, queries)
SELECT pid::uuid, month, q FROM unnest($1::text[], $2::date[], $3::int[]) AS t(pid, month, q)
ON CONFLICT (profile_id, month) DO UPDATE SET queries = usage_monthly.queries + EXCLUDED.queries`,
		mp, mm, mq); err != nil {
		return err
	}
	if len(dp) > 0 {
		if _, err := w.DB.Exec(ctx, `
INSERT INTO stats_domains_daily (profile_id, day, domain, status, hits)
SELECT pid::uuid, day, domain, status, hits FROM unnest($1::text[], $2::date[], $3::text[], $4::text[], $5::int[]) AS t(pid, day, domain, status, hits)
ON CONFLICT (profile_id, day, domain, status) DO UPDATE SET hits = stats_domains_daily.hits + EXCLUDED.hits`,
			dp, dday, dd, ds, dh); err != nil {
			return err
		}
	}

	// Живая лента: новые записи — в голову списка, храним последние 200.
	pipe := w.R.Pipeline()
	for pid, rows := range feed {
		for _, r := range rows {
			pipe.LPush(ctx, "recent:"+pid, r)
		}
		pipe.LTrim(ctx, "recent:"+pid, 0, 199)
	}
	_, err := pipe.Exec(ctx)
	return err
}
