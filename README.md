# DrNetwork Panel

**A multi-node web panel for sing-box** • Built on [S-UI](https://github.com/alireza0/s-ui) and [SagerNet/sing-box](https://github.com/SagerNet/sing-box)

[![Release](https://img.shields.io/github/v/release/Danialrostamani/drnetwork-panel.svg)](https://github.com/Danialrostamani/drnetwork-panel/releases/latest)
[![Go Report Card](https://goreportcard.com/badge/github.com/Danialrostamani/drnetwork-panel)](https://goreportcard.com/report/github.com/Danialrostamani/drnetwork-panel)
[![Downloads](https://img.shields.io/github/downloads/Danialrostamani/drnetwork-panel/total.svg)](https://github.com/Danialrostamani/drnetwork-panel/releases)
[![License](https://img.shields.io/badge/license-GPL%20V3-blue.svg?longCache=true)](https://www.gnu.org/licenses/gpl-3.0.en.html)

> Independent, full-source DrNetwork distribution. Backend and frontend are stored in this repository; no GitHub fork or submodule is required. Official S-UI changes are imported through a validated synchronization workflow.

> **Disclaimer:** This project is only for personal learning and communication, please do not use it for illegal purposes, please do not use it in a production environment

**If you think this project is helpful to you, you may wish to give a**:star2:

**Want to contribute?** See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, coding conventions, testing, and the pull request process.

## Quick Overview
| Features                               |      Enable?       |
| -------------------------------------- | :----------------: |
| Multi-Protocol                         | :heavy_check_mark: |
| Multi-Language                         | :heavy_check_mark: |
| Multi-Client/Inbound                   | :heavy_check_mark: |
| Multi-Node (one master, any number of nodes) | :heavy_check_mark: |
| Telegram bot (owner, roles, group-limited admins, per-admin volume limits) | :heavy_check_mark: |
| Advanced Traffic Routing Interface     | :heavy_check_mark: |
| Client & Traffic & System Status       | :heavy_check_mark: |
| Subscription Link (link/json/clash + info)| :heavy_check_mark: |
| Dark/Light Theme                       | :heavy_check_mark: |
| API Interface                          | :heavy_check_mark: |

## Supported Platforms
| Platform | Architecture | Status |
|----------|--------------|---------|
| Linux    | amd64, arm64, armv7, armv6, armv5, 386, s390x | ✅ Supported |
| Windows  | amd64, 386, arm64 | ✅ Supported |
| macOS    | amd64, arm64 | 🚧 Experimental |

## Default Installation Information
- Panel Port: 2095
- Panel Path: /app/
- Subscription Port: 2096
- Subscription Path: /sub/
- User/Password: admin

## Install & Upgrade to Latest Version

### Linux/macOS
```sh
bash <(curl -Ls https://raw.githubusercontent.com/Danialrostamani/drnetwork-panel/main/install.sh)
```

#### Installer language

The installer is available in the same six languages as the panel: `en` (default), `fa`, `ru`, `vi`, `zhcn`, `zhtw`. Choose one with the `SUI_LANG` environment variable (when unset, your system `$LANG` is used as a hint):

```sh
SUI_LANG=fa bash <(curl -Ls https://raw.githubusercontent.com/Danialrostamani/drnetwork-panel/main/install.sh)
```

### Alpine Linux
Alpine uses `apk` and OpenRC instead of `apt`/systemd. The install script detects Alpine automatically and sets up an OpenRC service. Since Alpine has no `bash` by default, install it first:

```sh
apk add bash
bash <(curl -Ls https://raw.githubusercontent.com/Danialrostamani/drnetwork-panel/main/install.sh)
```

Manage the service with OpenRC: `rc-service s-ui start|stop|restart` and `rc-update add s-ui default`.

### Windows
1. Download the latest Windows release from [GitHub Releases](https://github.com/Danialrostamani/drnetwork-panel/releases/latest)
2. Extract the ZIP file
3. Run `install-windows.bat` as Administrator
4. Follow the installation wizard

## Manual installation

### Linux/macOS
1. Get the latest version of DrNetwork based on your OS/Architecture from GitHub: [https://github.com/Danialrostamani/drnetwork-panel/releases/latest](https://github.com/Danialrostamani/drnetwork-panel/releases/latest)
2. **OPTIONAL** Get the latest version of `s-ui.sh` [https://raw.githubusercontent.com/Danialrostamani/drnetwork-panel/main/s-ui.sh](https://raw.githubusercontent.com/Danialrostamani/drnetwork-panel/main/s-ui.sh)
3. **OPTIONAL** Copy `s-ui.sh` to /usr/bin/ and run `chmod +x /usr/bin/s-ui`.
4. Extract the `s-ui-*.tar.gz` file to a directory of your choice and navigate to the directory where you extracted the tar.gz file.
5. Copy *.service files to /etc/systemd/system/ and run `systemctl daemon-reload`.
6. Enable autostart and start the DrNetwork service (named `s-ui`) using `systemctl enable s-ui --now`
7. Start sing-box service using `systemctl enable sing-box --now`

### Windows
1. Get the latest Windows version from GitHub: [https://github.com/Danialrostamani/drnetwork-panel/releases/latest](https://github.com/Danialrostamani/drnetwork-panel/releases/latest)
2. Download the appropriate Windows package (e.g., `s-ui-windows-amd64.zip`)
3. Extract the ZIP file to a directory of your choice
4. Run `install-windows.bat` as Administrator
5. Follow the installation wizard
6. Access the panel at http://localhost:2095/app

## Uninstall DrNetwork

### systemd
```sh
sudo -i

systemctl disable s-ui  --now

rm -f /etc/systemd/system/sing-box.service
systemctl daemon-reload

rm -fr /usr/local/s-ui
rm /usr/bin/s-ui
```

### Alpine (OpenRC)
```sh
rc-service s-ui stop
rc-update del s-ui default
rm -f /etc/init.d/s-ui

rm -fr /usr/local/s-ui
rm /usr/bin/s-ui
```

## Install using Docker

<details>
   <summary>Click for details</summary>

### Usage

**Step 1:** Install Docker

```shell
curl -fsSL https://get.docker.com | sh
```

**Step 2:** Install DrNetwork

> Docker compose method

```shell
mkdir s-ui && cd s-ui
wget -q https://raw.githubusercontent.com/Danialrostamani/drnetwork-panel/main/docker-compose.yml
docker compose up -d
```

> Use docker

```shell
mkdir s-ui && cd s-ui
docker run -itd \
    -p 2095:2095 -p 2096:2096 -p 443:443 -p 80:80 \
    -v $PWD/db/:/app/db/ \
    -v $PWD/cert/:/root/cert/ \
    --name s-ui --restart=unless-stopped \
    ghcr.io/danialrostamani/drnetwork-panel:latest
```

> Build your own image

```shell
git clone https://github.com/Danialrostamani/drnetwork-panel
cd drnetwork-panel
docker build -t drnetwork-panel .
```

</details>

## Manual run ( contribution )

<details>
   <summary>Click for details</summary>

### Build and run whole project
```shell
./runSUI.sh
```

### Clone the repository
```shell
# Backend and frontend are included in one repository
git clone https://github.com/Danialrostamani/drnetwork-panel
cd drnetwork-panel
```


### - Frontend

The complete DrNetwork frontend source is included in [`frontend/`](frontend/).

### - Backend
> Please build frontend once before!

To build backend:
```shell
# remove old frontend compiled files
rm -fr web/html/*
# apply new frontend compiled files
cp -R frontend/dist/ web/html/
# build
go build -o sui main.go
```

To run backend (from root folder of repository):
```shell
./sui
```

</details>

## Languages

- English
- Farsi
- Vietnamese
- Chinese (Simplified)
- Chinese (Traditional)
- Russian

## Features

- Supported protocols:
  - General:  Mixed, SOCKS, HTTP, HTTPS, Direct, Redirect, TProxy
  - V2Ray based: VLESS, VMess, Trojan, Shadowsocks
  - Other protocols: ShadowTLS, Hysteria, Hysteria2, Naive, TUIC
- Supports XTLS protocols
- An advanced interface for routing traffic, incorporating PROXY Protocol, External, and Transparent Proxy, SSL Certificate, and Port
- An advanced interface for inbound and outbound configuration
- Clients’ traffic cap and expiration date
- Multi-node: after adding a node or an inbound, one click on the Clients page (🛠 menu → *Add all inbounds to all clients*) attaches every inbound that takes clients - node-hosted ones included - to every client
- Top-bar badge with the number of nodes that are online, on every page
- Displays online clients, inbounds and outbounds with traffic statistics, and system status monitoring
- Subscription service with ability to add external links and subscription
- Several subscription domains at once (new links use the first, the others keep answering) and wildcard domains (`*.sub.example.com`, with a wildcard DNS record and certificate) that give every client a host of its own, so a blocked host costs one client only. Inbound addresses take wildcards too (`*.cdn.example.com`), and an address marked *Behind CDN* refuses what a CDN cannot carry (REALITY, QUIC, raw TCP)
- Link name template with variables (`{USER}`, `{PROTOCOL}`, `{NODE}`, `{REMAINING}`, `{DAYS_LEFT}`, `{EXPIRE_JALALI}` and more) and a live preview, plus the announcement, support link and account page button that Happ and v2RayTun show with the subscription
- Decoy site: a folder of your own is served outside the panel path, so the server looks like an ordinary website
- Telegram shop with plans and a wallet: discount codes limited to plans, kinds of order or once per customer, gift codes, auto-renewal from the wallet, and card-to-card payments approved on their own from the bank's deposit SMS (**Shop → SMS**: an SMS forwarder app posts each message to the panel; a unique amount per order tells the orders apart, and a receipt already used is refused)
- HTTPS for secure access to the web panel and subscription service (self-provided domain + SSL certificate)
- Dark/Light theme

## Nodes

A node is another DrNetwork panel that the master monitors and keeps in sync. Open **Nodes** on the master:

- **Add a node** - *New server* gives a one-line command to run as root on a fresh server; it installs the latest release with a random panel port, path and API token for this master, then the wizard waits for the node to answer. *Existing panel* takes the address and an API token made on that panel (**Admins → API token**, or `s-ui token -add <token> -desc master` on its shell). **Add several nodes** takes one `name URL token [path]` per line. For security the master never logs in to your servers over SSH or asks for their passwords: you run the command yourself.
  ```sh
  bash <(curl -Ls https://raw.githubusercontent.com/Danialrostamani/drnetwork-panel/main/install.sh) --node-token <token> --port <port> --path /<path>/
  ```
  The installer also takes `--version <version>`, such as `--version 32`.
- **Overview** - a summary bar (online, down, users online, live speed, today's traffic, warnings), search, status/tag/country filters, sorting, cards or a table, tags, a country flag and a manual order per node.
- **Each card** - status and how long it has been down, latency, CPU/RAM/disk, users online, speed, today's and this month's traffic, monthly cap, 24 h uptime, versions, warnings.
- **Details** - who is online, history charts (load, latency, users, traffic) up to 7 days, traffic per day or month with the clients that used the node most, uptime and outages, the node's logs and change history, an outbound test run by the node's own core, and the sync tab: what a sync would change, the last sync report, and a full sync that rewrites every client on the node.
- **Actions** - check now, sync, full sync, restart core or panel, maintenance on/off, enable/disable, clone, backup of a node's database or of all nodes in one zip; select several nodes to run an action on all of them.
- **Alerts** - per node CPU, RAM, disk, ping and certificate-expiry thresholds and an older-version warning, sent by the Telegram bot when passed for a while and again on recovery (empty = default, 0 = off).
- **Monthly cap** - the traffic the node's server may move in a month (upload, download or both) with a reset day; the bot warns at 80%, 90% and 100%, and the node's links can leave subscriptions once it is used up.
- **Subscriptions** - optionally leave a node's links out of subscriptions while it is down (off by default; after 3 minutes, back as soon as it answers).
- **Access** - limit a node to some client groups and/or clients: only they are synced to it and get its links. Empty (the default) serves every client.

Update the nodes together with the master: an older node shows less (no NIC traffic, disk, host or per-client figures).

## Telegram bot

DrNetwork includes an optional Telegram bot for administrators (**Settings → Telegram Bot**):

1. Create a bot with [@BotFather](https://t.me/BotFather) and paste its token.
2. Send `/id` to your bot, add the number it returns to **Admin Telegram IDs** and, if you are the one who manages the other admins, put the same number in **Bot owner Telegram ID** (see below).
3. Enable the bot. If the server cannot reach `api.telegram.org`, set a proxy (`http://` or `socks5://`).

Run the bot **on the master only** (nodes do not need it). Send `/menu` for the button panel — it mirrors the web panel:

| Menu | What you can do |
| --- | --- |
| 🏠 Home | CPU / RAM / disk / swap bars, uptime, IPs, network totals, sing-box state, totals, node health; restart core, maintenance, logs, backup |
| 👥 Clients | Filtered, paged list (all / active / disabled / near limit / depleted / online) in creation order - oldest first by default, a 🔃 button flips to newest first (the choice is remembered) - with each client's creation date and 🏷 group, search (name, note or group), **🏷 Groups screen** (every group with its client count; tap one to filter the list, or “no group”), new-client wizard (name → **group: pick an existing one, tap ➕ New group, or just type a new name** → volume **(buttons 10 20 30 / 50 100 200 / 300 ∞, or ✏️ Custom for any number)** → days → IP limit), **new client from a JSON template**, bulk create (`/addbulk` takes an optional group), cleanup of depleted clients. **Client card = the whole panel form:** enable/disable, reset traffic, ✏️ *Edit* (name, description, remark, group - same chooser: existing groups as buttons, ➕ New group, or a typed new name -, volume, days or exact expiry date, IP limit, delay start, auto reset, reset days), 🔑 *Config* (per-protocol credentials: regenerate one/all or replace by JSON), 🔗 *External & sub links* (add/delete), inbound assignment, subscription + QR, links, online IPs, kick, bind Telegram, `{ }` JSON edit of the whole client, delete. The card itself ends with the client's **subscription link** (tap it to copy). 🛠 *Bulk edit* on a filter or group: add volume/days, set IP limit, enable/disable, add/remove inbound, reset traffic, delete; plus 🔗 *Add all inbounds to all clients* (the panel's tools-menu button) |
| 📡 Inbounds · 📤 Outbounds · 🔌 Endpoints · 🛠 Services · 🔐 TLS · 🖥 Nodes | List, view, JSON view, create (templates or JSON; outbounds also from share links), edit by sending JSON (text or `.json` file), delete; inbound port / clients; outbound latency test; node enable, sync, probe |
| 📏 Rules · 🌐 DNS · ⚙️ Basics | Routing rules, rule sets, DNS servers/rules (add, edit, reorder, delete), route/DNS options, log level, NTP, experimental |
| 🔧 Settings | Panel and subscription settings (toggles and values), panel restart |
| 📊 Stats · 🧾 Changes · 📜 Logs · 👮 Admins | Traffic by user / inbound / outbound with a chart (the master's own numbers plus what the nodes counted - see below), change history with the admin who made each change, logs by level, admin list (the owner edits the admins here) |

Slash commands still work for quick use: `/add <name> <GB> <days> [ipLimit]`, `/addbulk <prefix> <count> <GB> <days> [ipLimit] [group]`, `/enable`, `/disable`, `/reset`, `/del`, `/volume`, `/expiry`, `/limitip`, `/sub`, `/bind`, `/unbind`, `/backup`, `/logs`, `/sync`, `/restart`, `/maintenance on|off`, `/home`, `/stats`, `/settings`, `/changes`.
Destructive actions (delete, reset, restart) ask for confirmation. Admin credentials and database restore are intentionally left to the web panel.

**Group-limited admins.** To give someone only their own clients, put one line per admin in **Settings → Telegram Bot → Admin group limits**: `TelegramID=Group name` (for example `12345678=Sales`). That admin then sees and manages only the clients of that group (matched ignoring case): list, search, online, create (always into their group - there is no group step), enable/disable, reset, delete, volume, expiry, IP limit, bind, subscription, bulk edit and cleanup inside the group. They cannot move a client to another group, and have no inbounds, outbounds, nodes, settings, logs, stats, backup, restart or admin list. Client alerts (volume/expiry) go only to the admin of that client's group; node, core and report alerts and the scheduled backup go to the full admins. A line alone is enough to make someone a limited admin (they do not need to be in **Admin Telegram IDs**), admins without a line keep full access, and a line that cannot be used (no `=`, no group, the reserved `@cluster` group) locks that ID out instead of leaving it unlimited. Changes apply within seconds.

**Owner and admin management.** Put your own numeric Telegram ID in **Settings → Telegram Bot → Bot owner Telegram ID**. The owner has every right and is the only one who can change the other admins, straight from the bot: **👮 Admins** lists everyone with their role, and tapping an admin opens ✏️ their card, where the owner switches sections on and off (Home, Clients, Inbounds, Outbounds, Endpoints, Services, TLS, Nodes, Routing, Settings, Stats, Logs, Core, Backup, Admins), applies a preset (full access, clients only, no access), limits the admin to one client group, or removes them. **➕ Add admin** takes a numeric ID (the person sends `/id` to the bot to read theirs) and a new admin starts with no access until sections are switched on. Nobody can edit the owner, and other admins - even with every section - cannot edit admins; the *Admins* section only lets them read the list. The owner is not built in: until the ID is set, nobody can edit admins from the bot. Think before handing out *Backup*: the file is the whole database, including panel credentials and tokens. What an admin may use shapes everything they get: the buttons and slash commands they see, and the alerts they receive (node alerts go to admins with *Nodes*, core alerts to *Home* or *Core*, client volume/expiry alerts to *Clients* or the client's group, the scheduled report is split between *Home* and *Stats*, the scheduled backup goes to *Backup*).

The roles are stored in three settings, so the web panel edits the same data as the bot: **Admin Telegram IDs** (who), **Admin group limits** (`ID=Group`, above) and **Admin section limits** (`12345678=clients,stats`; a line with nothing after the `=` means no access, unknown section names are ignored, and a group line wins over a sections line). An ID with neither line keeps full access, and a line that cannot be read leaves that ID with no access instead of widening it. Changes made in either place reach the running bot within seconds, without a restart.

**Volume limits.** The owner can cap how much traffic an admin's clients may use: **👮 Admins → ✏️ the admin → 📦 Volume limit**. Start from a ready amount, top up with **➕ 10 / 50 / 100 / 500 GB**, or type the total in GB (`500`, `2.5`; `+100` or `-50` change the current total). Nothing is deducted when a client is created or given volume: the admin's total goes down as their clients really use traffic. A traffic reset gives nothing back, and neither does deleting a client - what it used stays counted, and a new link counts only from its own first byte. A client counts for an admin when they created it through the bot while the limit is on, or when it is in the group of an admin limited to a group; clients an admin created before their limit existed are found in the history, and what any client used before it counted is never charged. When nothing is left the admin keeps managing their clients (disable, extend, lower a volume, delete) but cannot create a client or give one more volume; the clients that exist are not cut off. Unlimited volume and auto reset are allowed and counted by what they use. The owner is never limited, neither is an admin without a limit; **♾ Remove limit** lifts one (so does removing the admin), and a limit set later starts from nothing; **🔄 Reset used** counts from zero again. A limited admin sees what is left in their menu, on the home screen and above the client list; the owner sees it next to each admin in **👮 Admins** and on the limit screen, with what was used. Every change of a limit is written to the history under `quota`, and the limits, with what was used, are in the database, so backups carry them.

**Stats include the nodes.** A client served through a node moves its traffic through that node, which counts it. The 📊 Stats screen therefore asks every enabled node (in parallel, a few seconds at most) for its totals and adds them to the master's own: the users the master also has, and the inbounds it replicates from that node. A node's own outbounds and node-local users or inbounds stay private, and outbound traffic is the master's own because outbounds and endpoints are not shared with nodes. A node that cannot be counted is named at the bottom of the screen - *needs an update* for a node that predates this feature (**update the nodes together with the master**), *unreachable* for one that does not answer.

**Change history.** What an admin changes from the bot is recorded under `telegram:<their Telegram ID>`, so the 🧾 Changes screen and the panel's *Changes* dialog show who did it (older entries say plain `telegram`; the dialog's `telegram` filter finds both).

**Subscription message.** The 🔗 *Subscription / QR* button and `/sub` send the subscription link with its QR code, and the link comes under a summary of the client's account: state, usage (with the bar and the volume), what is left of the volume and of the time, the IP limit, when the client was last online and when it was created. The message can therefore be passed on to the client as it is; it holds nothing for administrators only (no note, group or Telegram binding), and a bound client gets the same message from `/sub`. The client card shows the same link on its last line, so it can be copied without opening the message.

**Bound clients** (`/bind <name> <telegramId>`) can use `/usage` (volume, expiry, status and their subscription link, with 🔗 *Subscription / QR* and 🔄 *Refresh* buttons under it), `/sub` (link + QR) and `/id`, and receive their own volume/expiry alerts. Everybody else only gets `/id`.

With **Alerts** enabled the bot reports node down/recovery, the master core stopping, and clients near their volume or expiry limit. **Scheduled report** accepts a cron spec (e.g. `@daily` or `0 9 * * *`) and posts the status and traffic summary, optionally with a database backup file. Changes in settings apply within seconds, without a restart.

## Environment Variables

<details>
  <summary>Click for details</summary>

### Usage

| Variable       |                      Type                      | Default       |
| -------------- | :--------------------------------------------: | :------------ |
| SUI_LOG_LEVEL  | `"debug"` \| `"info"` \| `"warn"` \| `"error"` | `"info"`      |
| SUI_DEBUG      |                   `boolean`                    | `false`       |
| SUI_BIN_FOLDER |                    `string`                    | `"bin"`       |
| SUI_DB_FOLDER  |                    `string`                    | `"db"`        |
| SINGBOX_API    |                    `string`                    | -             |

</details>

## SSL Certificate

<details>
  <summary>Click for details</summary>

### Certbot

```bash
snap install core; snap refresh core
snap install --classic certbot
ln -s /snap/bin/certbot /usr/bin/certbot

certbot certonly --standalone --register-unsafely-without-email --non-interactive --agree-tos -d <Your Domain Name>
```

</details>

## Third-party Projects

Community-made tools built around S-UI, the project DrNetwork is based on. They are not affiliated with or maintained by DrNetwork or S-UI, and some may rely on S-UI behaviour that DrNetwork has changed - use them at your own discretion:

- [itning/reset-s-ui-traffic](https://github.com/itning/reset-s-ui-traffic) — periodic traffic reset for all users
- [zqh2333/s-ui-traffic-reset](https://github.com/zqh2333/s-ui-traffic-reset) — traffic reset tool
- [Sownix21/SUI-Bot](https://github.com/Sownix21/SUI-Bot) - telegram bot

## Stargazers over Time
[![Stargazers over time](https://starchart.cc/Danialrostamani/drnetwork-panel.svg)](https://starchart.cc/Danialrostamani/drnetwork-panel)

## Thanks

DrNetwork Panel would not exist without the work of others:

- **[Alireza (alireza0)](https://github.com/alireza0) and the contributors of [S-UI](https://github.com/alireza0/s-ui)** - the panel, the multi-inbound-per-user design and the frontend this project is built on. Thank you for sharing it as open source.
- **[SagerNet/sing-box](https://github.com/SagerNet/sing-box)** - the proxy core.
- Everyone who reports bugs, tests releases and contributes code.

The S-UI [wiki](https://github.com/alireza0/s-ui/wiki) remains a good reference for the parts DrNetwork keeps as they are (DrNetwork adds the nodes, the Telegram bot and their settings on top):

| Page | Contents |
|------|----------|
| [Subscription Service](https://github.com/alireza0/s-ui/wiki/Subscription-Service) | Subscription URLs, the three formats, response headers |
| [Subscription JSON Template](https://github.com/alireza0/s-ui/wiki/Subscription-JSON-Template) | Structure and supported keys of the sing-box subscription template |
| [Subscription Clash Template](https://github.com/alireza0/s-ui/wiki/Subscription-Clash-Template) | The Clash.Meta template, proxy groups and filters |
| [API Documentation](https://github.com/alireza0/s-ui/wiki/API-Documentation) | The token-authenticated REST API (`/apiv2`) |
| [Configuration Objects](https://github.com/alireza0/s-ui/wiki/Configuration-Objects) | Shape of the objects read and written through the API |
| [Settings Reference](https://github.com/alireza0/s-ui/wiki/Settings-Reference) | Every panel setting and its default |

DrNetwork is released under the same license as S-UI, GPL-3.0 - see [LICENSE](LICENSE).
