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

-- name: GetDashboardConfig :one
SELECT ideal_max, unsafe_min, chart_min, chart_max
FROM dashboard_config
WHERE id = 1;

-- name: UpsertDashboardConfig :exec
INSERT INTO dashboard_config (
  id, ideal_max, unsafe_min, chart_min, chart_max, updated_unix_ms
) VALUES (1, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  ideal_max = excluded.ideal_max,
  unsafe_min = excluded.unsafe_min,
  chart_min = excluded.chart_min,
  chart_max = excluded.chart_max,
  updated_unix_ms = excluded.updated_unix_ms;
