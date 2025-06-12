# Vivteno - Website Health Monitor

Vivteno is a terminal-based tool for monitoring the health and availability of a website. It periodically pings a target website and can optionally fetch and display data from a health endpoint.

## Features

- Periodic TCP ping to a specified website.
- Optional health endpoint check (expects JSON).
- Customizable schedule and timezone.
- Colorful, user-friendly terminal UI (Bubble Tea + Lipgloss).

## Requirements

- Go 1.24 or newer

## Building

Clone the repository and build with:

```sh
git clone https://github.com/mooship/vivteno.git
cd vivteno
go build -o vivteno
```

## Configuration

Create a `.env` file in the project directory with the following variables:

```
PING_WEBSITE=example.com
PING_SCHEDULE=10s
TIMEZONE=UTC
HEALTH_ENDPOINT=/health
```

- `PING_WEBSITE`: Hostname or IP to monitor (required).
- `PING_SCHEDULE`: Interval between checks (e.g., `10s`, `1m`). Default: `10s`.
- `TIMEZONE`: (Optional) Timezone for timestamps (e.g., `UTC`, `America/New_York`).
- `HEALTH_ENDPOINT`: (Optional) Path to health endpoint (e.g., `/health`).

## Running

After building and configuring:

```sh
./vivteno
```

Press `q` or `Ctrl+C` to quit.

## License

This project is open source and available under the GNU General Public License v3.0 (GPL-3.0).

See [LICENSE](./LICENSE) for details.
