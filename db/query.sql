-- name: InsertReading :exec
INSERT INTO readings (
  time_unix_ms, value, display, unit, range_low, range_high, overload, max_min,
  low_power, auto_power_off, backlight, hold
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListRecentReadings :many
SELECT time_unix_ms, value, display, unit, range_low, range_high, overload, max_min,
       low_power, auto_power_off, backlight, hold
FROM readings
WHERE time_unix_ms >= ?
ORDER BY time_unix_ms DESC
LIMIT ?;
