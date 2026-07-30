# sgtm

Bluetooth Low Energy scanner/reader for discovering a sound level meter and
inspecting its GATT data.

## Usage

Build with the macOS Bluetooth privacy usage string embedded:

```sh
mage
```

Build and open the native macOS app bundle:

```sh
mage openApp
```

Run the Chrome Web Bluetooth dashboard:

```sh
./bin/sgtm
```

Run the native Go/CoreBluetooth dashboard from the CLI:

```sh
./bin/sgtm native
```

Scan nearby BLE advertisements:

```sh
mage scan
```

Inspect a device by advertised name or CoreBluetooth UUID:

```sh
ARGS='--name decibel --notify 60s' mage inspect
ARGS='--addr XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX --notify 60s' mage inspect
ARGS='--addr XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX --notify 60s --write d5fc110d' mage inspect
```

Run either dashboard with custom flags:

```sh
./bin/sgtm --listen :8090
./bin/sgtm native --name "850019 EM" --listen :8080
```

Chrome will prompt for Bluetooth access when you click Connect. The native
commands may prompt for Bluetooth permission for the terminal or host app that
launches them.

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

The default `sgtm` command serves a Chrome Web Bluetooth dashboard. Chrome
connects directly to the meter, so click Connect in the page and select
`850019 EM` from the Bluetooth picker.

The Chrome dashboard stores recent history in IndexedDB. The native `.app`
bundle runs the Go/CoreBluetooth reader directly; the native CLI command
launches the same reader in Terminal.app. Both native forms serve a local
browser display with SQLite history:

- current dBA/dBC reading
- rolling history chart
- green ideal band up to `--ideal-max` (default `85`)
- red too-high band starting at `--unsafe-min` (default `95`)
- SQLite history saved to `--db`

The default history window is 30 minutes. Adjust it with `--history`, for
example `--history 2h`.

Regenerate the typed SQLite package after changing `db/schema.sql` or
`db/query.sql`:

```sh
mage sqlc
```
