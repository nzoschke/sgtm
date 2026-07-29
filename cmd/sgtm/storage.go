package main

import (
	"context"
	"database/sql"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type readingDB struct {
	db *sql.DB
}

func openReadingDB(path string) (*readingDB, error) {
	db, err := sql.Open("sqlite3", path+"?_busy_timeout=5000&_journal_mode=WAL")
	if err != nil {
		return nil, err
	}
	rdb := &readingDB{db: db}
	if err := rdb.init(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return rdb, nil
}

func (r *readingDB) Close() error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.Close()
}

func (r *readingDB) init(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS readings (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  time_unix_ms INTEGER NOT NULL,
  value REAL NOT NULL,
  display REAL NOT NULL,
  unit TEXT NOT NULL,
  range_low INTEGER NOT NULL,
  range_high INTEGER NOT NULL,
  overload TEXT NOT NULL,
  max_min TEXT NOT NULL,
  low_power INTEGER NOT NULL,
  auto_power_off INTEGER NOT NULL,
  backlight INTEGER NOT NULL,
  hold INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS readings_time_idx ON readings(time_unix_ms);
`)
	return err
}

func (r *readingDB) Insert(ctx context.Context, reading soundReading) error {
	if r == nil {
		return nil
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO readings (
  time_unix_ms, value, display, unit, range_low, range_high, overload, max_min,
  low_power, auto_power_off, backlight, hold
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		reading.Time.UnixNano()/int64(time.Millisecond),
		reading.Value,
		reading.Display,
		reading.Unit,
		reading.RangeLow,
		reading.RangeHigh,
		reading.Overload,
		reading.MaxMin,
		boolInt(reading.LowPower),
		boolInt(reading.AutoPowerOff),
		boolInt(reading.Backlight),
		boolInt(reading.Hold),
	)
	return err
}

func (r *readingDB) Recent(ctx context.Context, since time.Time, limit int) ([]soundReading, error) {
	if r == nil {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT time_unix_ms, value, display, unit, range_low, range_high, overload, max_min,
       low_power, auto_power_off, backlight, hold
FROM readings
WHERE time_unix_ms >= ?
ORDER BY time_unix_ms DESC
LIMIT ?`, since.UnixNano()/int64(time.Millisecond), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reversed []soundReading
	for rows.Next() {
		var ms int64
		var reading soundReading
		var lowPower, autoPowerOff, backlight, hold int
		if err := rows.Scan(
			&ms,
			&reading.Value,
			&reading.Display,
			&reading.Unit,
			&reading.RangeLow,
			&reading.RangeHigh,
			&reading.Overload,
			&reading.MaxMin,
			&lowPower,
			&autoPowerOff,
			&backlight,
			&hold,
		); err != nil {
			return nil, err
		}
		reading.Time = time.Unix(0, ms*int64(time.Millisecond))
		reading.LowPower = lowPower != 0
		reading.AutoPowerOff = autoPowerOff != 0
		reading.Backlight = backlight != 0
		reading.Hold = hold != 0
		reversed = append(reversed, reading)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	return reversed, nil
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
