# sgtm

Bluetooth Low Energy scanner/reader for discovering a sound level meter and
inspecting its GATT data.

## Usage

Build with the macOS Bluetooth privacy usage string embedded:

```sh
make build
```

Scan nearby BLE advertisements:

```sh
bin/sgtm scan --duration 20s
```

Inspect a device by advertised name or CoreBluetooth UUID:

```sh
bin/sgtm inspect --name decibel --notify 60s
bin/sgtm inspect --addr XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX --notify 60s
bin/sgtm inspect --addr XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX --notify 60s --write d5fc110d
```

Run the full-screen booth dashboard:

```sh
bin/sgtm dashboard --name "850019 EM" --listen :8080 --db .context/sgtm.sqlite
```

On macOS, the first run may prompt for Bluetooth permission for the terminal or
host app that launches the command.

## 850019 notes

The Sper Scientific 850019 manual says the meter powers off after about 10
minutes with no button presses. While it is on, hold the physical POWER button
for 2 seconds to disable auto power off until the next time it is turned on.

To advertise over Bluetooth, press the Bluetooth button on the meter. The meter
appears as `850019 EM` here and exposes service `fff0`, writable characteristic
`fff1`, and notify characteristic `fff2`.

The Android Meterbox Pro app maps `850019 EM` to the environmental-meter `DT95`
protocol. Send `d5fc110d` to `fff1` after enabling notifications on `fff2` to
start the live dBA/dBC stream.

## Dashboard

The `dashboard` command serves a local browser display with:

- current dBA/dBC reading
- rolling history chart
- green ideal band up to `--ideal-max` (default `85`)
- red too-high band starting at `--unsafe-min` (default `95`)
- SQLite history saved to `--db`

The default history window is 30 minutes. Adjust it with `--history`, for
example `--history 2h`.
