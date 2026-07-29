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
