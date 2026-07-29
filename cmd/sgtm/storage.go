package main

import (
	"context"
	"database/sql"
	"time"

	meterdb "github.com/nzoschke/sgtm/db"

	_ "github.com/mattn/go-sqlite3"
)

type readingDB struct {
	db      *sql.DB
	queries *meterdb.Queries
}

func openReadingDB(path string) (*readingDB, error) {
	db, err := sql.Open("sqlite3", path+"?_busy_timeout=5000&_journal_mode=WAL")
	if err != nil {
		return nil, err
	}
	rdb := &readingDB{db: db, queries: meterdb.New(db)}
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
	_, err := r.db.ExecContext(ctx, meterdb.Schema)
	return err
}

func (r *readingDB) Insert(ctx context.Context, reading soundReading) error {
	if r == nil {
		return nil
	}
	return r.queries.InsertReading(ctx, meterdb.InsertReadingParams{
		TimeUnixMs:   reading.Time.UnixNano() / int64(time.Millisecond),
		Value:        reading.Value,
		Display:      reading.Display,
		Unit:         reading.Unit,
		RangeLow:     int64(reading.RangeLow),
		RangeHigh:    int64(reading.RangeHigh),
		Overload:     reading.Overload,
		MaxMin:       reading.MaxMin,
		LowPower:     boolInt(reading.LowPower),
		AutoPowerOff: boolInt(reading.AutoPowerOff),
		Backlight:    boolInt(reading.Backlight),
		Hold:         boolInt(reading.Hold),
	})
}

func (r *readingDB) Recent(ctx context.Context, since time.Time, limit int) ([]soundReading, error) {
	if r == nil {
		return nil, nil
	}
	rows, err := r.queries.ListRecentReadings(ctx, meterdb.ListRecentReadingsParams{
		TimeUnixMs: since.UnixNano() / int64(time.Millisecond),
		Limit:      int64(limit),
	})
	if err != nil {
		return nil, err
	}

	reversed := make([]soundReading, 0, len(rows))
	for _, row := range rows {
		reversed = append(reversed, soundReading{
			Time:         time.Unix(0, row.TimeUnixMs*int64(time.Millisecond)),
			Value:        row.Value,
			Display:      row.Display,
			Unit:         row.Unit,
			RangeLow:     int(row.RangeLow),
			RangeHigh:    int(row.RangeHigh),
			Overload:     row.Overload,
			MaxMin:       row.MaxMin,
			LowPower:     row.LowPower != 0,
			AutoPowerOff: row.AutoPowerOff != 0,
			Backlight:    row.Backlight != 0,
			Hold:         row.Hold != 0,
		})
	}
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	return reversed, nil
}

func boolInt(v bool) int64 {
	if v {
		return 1
	}
	return 0
}
