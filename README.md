# DrNetwork Panel

**A multi-node web panel for sing-box** • Built on [S-UI](https://github.com/alireza0/s-ui) and [SagerNet/sing-box](https://github.com/SagerNet/sing-box)

[![Release](https://img.shields.io/github/v/release/Danialrostamani/drnetwork-panel.svg)](https://github.com/Danialrostamani/drnetwork-panel/releases/latest)
[![Go Report Card](https://goreportcard.com/badge/github.com/Danialrostamani/drnetwork-panel)](https://goreportcard.com/report/github.com/Danialrostamani/drnetwork-panel)
[![Downloads](https://img.shields.io/github/downloads/Danialrostamani/drnetwork-panel/total.svg)](https://github.com/Danialrostamani/drnetwork-panel/releases)
[![License](https://img.shields.io/badge/license-GPL%20V3-blue.svg?longCache=true)](https://www.gnu.org/licenses/gpl-3.0.en.html)

> Independent, full-source DrNetwork distribution. Backend and frontend are stored in this repository; no GitHub fork or submodule is required. The panel is developed and released from this repository alone: selected upstream fixes are reviewed and integrated into the local source, while installs, updates and the Docker image come only from this repository's releases.

> **Disclaimer:** This project is only for personal learning and communication, please do not use it for illegal purposes, please do not use it in a production environment

**If you think this project is helpful to you, you may wish to give a**:star2:

**Want to contribute?** See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, coding conventions, testing, and the pull request process.

## Stable hardware-aware balancing (v52.4)

Enable **Settings → Subscription → node assignment → Hardware-aware stable balancing**.
The option is off by default: upgrading does not change an existing percentage policy.
Only listed managed nodes that already serve the client participate; access rules,
private subscription tokens and non-node/manual links are preserved.

- CPU cores and RAM are read from node probes. The automatic baseline is
  `min(cores, RAM GiB / 2)` (minimum 0.25 units; missing hardware uses a conservative
  one-core baseline). This is a heuristic, not a CPU-model benchmark. An optional
  **capacity override** replaces it. The existing share becomes a relative capacity
  multiplier, not an exact percentage, in smart mode; zero is emergency-only.
- Enter **measured network capacity in Mbps**, not advertised NIC/port speed.
  Empty means unknown and is omitted from scoring. Client up/down rates use the
  busier direction, not their sum (full duplex). An optional online admission
  threshold is sampled, not a hard live connection limit.
- New assignments use hardware capacity × multiplier × free headroom × remaining
  monthly quota × recovery ramp. CPU, memory and network rates use a time-based
  one-minute EWMA. This is server-side capacity, not ISP-specific user latency.
- Weighted rendezvous hashing spreads primary and backup choices. The primary is
  stored in the database (and backups), survives panel restart and is not re-picked
  because a load reading or multiplier changes. Allocation happens on first
  subscription generation, not at account creation. Assigned accounts are not
  the same as active connections.
- New users avoid disabled, draining, busy, exhausted, stale (>120 s) or unconfirmed
  nodes. At least two healthy probes are required. One failed probe stops new
  assignments but allows an existing choice a short grace (at most 30 s); a second
  failed probe removes that grace. Full traffic caps apply even if manual link
  hiding is disabled. Access permissions always limit the available set.
- Confirmed failure redistributes affected users among eligible nodes instead of
  concentrating them on the first list entry. Recovered nodes ramp new admissions
  over five minutes; users that moved away are not automatically called back.
- Sustained overload for at least two minutes can move a deterministic roughly
  10% cohort per minute, only when that user fetches a profile and another normal
  node is eligible. A ten-minute per-assignment cooldown prevents repeated moves.
- **Drain** stops new assignments without evicting existing ones or closing sockets.
  It does not forcibly migrate existing users. Settings show hardware, eligibility,
  relative capacity/free score and counts of assigned enabled accounts.

### What Sing-box actually receives

In smart mode, `proxy` defaults to `auto`. That `urltest` contains only compatible
outbounds on the assigned node, so a globally low-ping node cannot silently defeat
the capacity distribution. A separate `backup` choice tests the first eligible
per-user backup node; other locations and local/manual links remain selectable.
Generated group tags are collision-safe. If no eligible assignment or compatible
assigned-node protocol exists, profile generation fails clearly instead of emitting
an unintended direct connection or an invalid empty automatic group.

**Stock Sing-box has no weighted, strict-priority failover group.** Backup is a
separate user choice, not an instant automatic primary→backup switch. Panel health
failover/rebalancing takes effect only after the client refreshes the JSON profile;
this does not move open connections without interruption. Existing cached/manual
selector choices or a custom template routing `final` elsewhere may override the
new default. Use the generated JSON profile and select `proxy → auto`; refresh
profiles after enabling this mode. URI-only subscriptions cannot enforce groups.

Back up the database/settings before upgrading. The new assignment table is included
in backups and removed for deleted clients. No deployment to your server, real
traffic benchmark or ISP-path measurement is implied by unit/browser tests.

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
  - Other protocols: ShadowTLS, Hysteria, Hysteria2 (Salamander or Gecko obfuscation, BBR profile), Naive, TUIC, AnyTLS, Snell, Cloudflared tunnel (optionally through an outbound such as WARP)
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
- HTTPS for secure access to the web panel and subscription service (self-provided domain + SSL certificate)
- Dark/Light theme

## Nodes

A node is another DrNetwork panel that the master monitors and keeps in sync. Open **Nodes** on the master:

- **Add a node** - *New server* gives a one-line command to run as root on a fresh server; it installs the latest release with a random panel port, path and API token for this master, then the wizard waits for the node to answer. *Existing panel* takes the address and an API token made on that panel (**Admins → API token**, or `s-ui token -add <token> -desc master` on its shell). **Add several nodes** takes one `name URL token [path]` per line. For security the master never logs in to your servers over SSH: you run the command yourself.
  ```sh
  bash <(curl -Ls https://raw.githubusercontent.com/Danialrostamani/drnetwork-panel/main/install.sh) --node-token <token> --port <port> --path /<path>/
  ```
  The installer also takes `--version <version>`, such as `--version 32`.
- **Overview** - a summary bar (online, down, users online, live speed, today's traffic, warnings), search, status/tag/country filters, sorting, cards or a table, tags, a country flag and a manual order per node.
- **Each card** - status and how long it has been down, latency, CPU/RAM/disk, users online, speed, today's and this month's traffic, monthly cap, 24 h uptime, versions, warnings.
- **Details** - who is online, history charts (load, latency, users, traffic) up to 7 days, traffic per day or month with the clients that used the node most, uptime and outages, the node's logs and change history, an outbound test run by the node's own core, and the sync tab: what a sync would change, the last sync report, and a full sync that rewrites every client on the node.
- **Actions** - check now, sync, full sync, restart core or panel, update panel, maintenance on/off, enable/disable, clone, backup of a node's database or of all nodes in one zip; select several nodes to run an action on all of them.
- **Update the nodes' panels from the master** - nodes older than the master get an arrow next to their version and an **Update panels (N)** button at the top of the Nodes page (the tools menu updates every node). Each node runs its own update to the latest release, as its Update button does, and the page shows the version it is going to until the node is back on it. Nodes from v34 on update through their API token; a node on v31 to v33 updates with its panel's username and password, asked once for all such nodes and never saved (a node with two-factor login takes its code when updated alone); v30 and older are updated on the node.
- **Alerts** - per node CPU, RAM, disk, ping and certificate-expiry thresholds and an older-version warning, sent by the Telegram bot when passed for a while and again on recovery (empty = default, 0 = off).
- **Monthly cap** - the traffic the node's server may move in a month (upload, download or both) with a reset day; the bot warns at 80%, 90% and 100%, and the node's links can leave subscriptions once it is used up.
- **Subscriptions** - optionally leave a node's links out of subscriptions while it is down (off by default; after 3 minutes, back as soon as it answers).
- **Assign users by weight** - **Settings → Subscription**: each user gets a default node from the weights you set; the other listed locations stay in the subscription so they can pick one themselves. Nodes not in the list are hidden. This replaces least-loaded-first.
- **Access** - limit a node to some client groups and/or clients: only they are synced to it and get its links. Empty (the default) serves every client.

Update the nodes together with the master: an older node shows less (no NIC traffic, disk, host or per-client figures).

## Live subscription page (v52.3)

The browser page on a subscription link updates account usage every second while visible, without a page reload. The old direct-message and refresh buttons have been removed from its header; support remains in its dedicated section. Polling pauses when the page is hidden/offline or left, resumes on return, avoids overlapping requests and backs off on errors. A failed update marks the displayed data offline until the connection recovers. Reset/renewal, enable/disable, volume and expiry changes are reflected without forcing counters to increase. Live updates do not restart numeric animations or regenerate QR codes every second.

The lightweight `?format=live` response contains account counters only, with `Cache-Control: no-store`. Local undrained core traffic and traffic awaiting a failed database commit are included under the accounting lock, without draining counters or writing statistics. Remote-node usage still follows the node collection interval: one-second page updates do not imply one-second remote-node sampling. Cached historical/status reports keep their existing lower-frequency schedule.

Configure **Settings → Subscription → Status page: look and sections → Web support direct-message URL**. The default is `https://t.me/DrNetWorkSingBox?direct`. This setting takes priority over seller/bot support routing on the web page; seller channel, shop and renewal behavior remain separate. Reports are prepared and copied locally, then the support direct opens. The user must paste and send the message in Telegram; the website never claims to have sent it. A selectable report preview is available if clipboard access fails. No subscription token or URL is automatically included in the report. The old `POST ?format=report` endpoint returns 410 rather than forwarding reports from cached pages to the bot. Channel direct messages must be enabled in Telegram for the target channel; setting a URL cannot enable that Telegram feature.

## Telegram bot

DrNetwork includes an optional Telegram bot for administrators. Everything about it is on one panel page, **Telegram bot**, with six tabs: *Dashboard* (bot status, pending orders, sales, sales by seller), *Orders*, *Sellers & admins* (add or remove a seller, their role or group, volume limit, terms, settlement and own shop), *Customers* (edit a customer's note, bound configs, introducer, seller and blocking; reset their test account so they can take a new one; delete a customer, with or without their configs), *Shop* and *Bot settings*. In the bot, the admin menu is grouped: 🏠 Server status, 👥 Clients, 🛍 Sales (orders, sales report, edit shop, customer view), 📊 Stats, 🔧 Technical, 🧰 Upkeep, 🔐 Admin access and 👑 Owner panel; the bottom keyboard keeps four buttons (Clients, Sales, Stats, Admin menu).

1. Create a bot with [@BotFather](https://t.me/BotFather) and paste its token.
2. Send `/id` to your bot, add the number it returns to **Admin Telegram IDs** and, if you are the one who manages the other admins, put the same number in **Bot owner Telegram ID** (see below).
3. Enable the bot. If the server cannot reach `api.telegram.org`, set a proxy (`http://` or `socks5://`).

Run the bot **on the master only** (nodes do not need it). Send `/menu` for the button panel — it mirrors the web panel. A **bottom menu** (🏠 Home, 📋 Main menu, 👥 Clients, 🖥 Nodes, 🛒 Shop admin, 📊 Stats - the ones your role opens) stays under the chat; it comes with `/start`, the first `/menu` and the bot's start message after an update, and customers get theirs with their next message.

Customers can **Connect link** and paste an existing subscription URL so the bot binds it to their Telegram account. Buy / renew / wallet requests go only to the owner (or full admins if no owner is set) with Approve / Reject; Approve creates or extends the client from the plan. A test account is created at once. If introducers are set, a **new customer cannot buy until they send that word**; each introducer is tied to a client group. Edit plans, introducers, payment details and the trial from **Telegram bot → Shop** or **Edit shop** in Telegram.

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

**Group-limited admins.** To give someone only their own clients, open **Telegram bot → Sellers & admins** and choose *Seller (group)* for them, or press *Own group for every admin* (the setting behind it holds one `TelegramID=Group name` line per admin). That admin then sees and manages only the clients of that group (matched ignoring case): list, search, online, create (always into their group - there is no group step), enable/disable, reset, delete, volume, expiry, IP limit, bind, subscription, bulk edit and cleanup inside the group. They cannot move a client to another group, and have no inbounds, outbounds, nodes, settings, logs, stats, backup, restart or admin list. Client alerts (volume/expiry) go only to the admin of that client's group; node, core and report alerts and the scheduled backup go to the full admins. A line alone is enough to make someone a limited admin (they do not need to be in **Admin Telegram IDs**), admins without a line keep full access, and a line that cannot be used (no `=`, no group, the reserved `@cluster` group) locks that ID out instead of leaving it unlimited. Changes apply within seconds.

**Owner and admin management.** Put your own numeric Telegram ID in **Telegram bot → Bot settings → Bot owner Telegram ID**. The owner has every right and, together with full administrators (trusted partners), can change the other admins straight from the bot: **🔐 Admin access** (or the panel's *Sellers & admins* tab) lists everyone with their role, and tapping an admin opens ✏️ their card, where the owner switches sections on and off (Home, Clients, Inbounds, Outbounds, Endpoints, Services, TLS, Nodes, Routing, Settings, Stats, Logs, Core, Backup, Admins), applies a preset (full access, clients only, no access), limits the admin to one client group, or removes them. **➕ Add admin** takes a numeric ID (the person sends `/id` to the bot to read theirs) and a new admin starts with no access until sections are switched on. The owner's administrator entry cannot be edited. Full administrators may edit other admins; a custom section checklist, even with every section selected, does not gain this authority. If no owner is configured, a full administrator's first access edit claims ownership; once set, ordinary partner edits do not change the owner. Think before handing out *Backup*: the file is the whole database, including panel credentials and tokens. What an admin may use shapes everything they get: the buttons and slash commands they see, and the alerts they receive (node alerts go to admins with *Nodes*, core alerts to *Home* or *Core*, client volume/expiry alerts to *Clients* or the client's group, the scheduled report is split between *Home* and *Stats*, the scheduled backup goes to *Backup*).

The roles are stored in three settings, so the web panel edits the same data as the bot: **Admin Telegram IDs** (who), **Admin group limits** (`ID=Group`, above) and **Admin section limits** (`12345678=clients,stats`; a line with nothing after the `=` means no access, unknown section names are ignored, and a group line wins over a sections line). An ID with neither line keeps full access, and a line that cannot be read leaves that ID with no access instead of widening it. Changes made in either place reach the running bot within seconds, without a restart.

**Private administration (v52.2, enabled by default).** In **Telegram bot → Bot settings → Owner & trusted partners**, save your own numeric Telegram ID first. Then enter a trusted person's numeric Telegram ID in the dedicated partner box and press **Grant full access**. The grant is immediate, confirmed explicitly, and audited; the person must start the bot to receive its messages. Owner and full partners get every bot tool even if the old panel-management switch was off. Other administrators retain only their permitted client tools and own shop; global commands, callbacks, wizards, technical alerts and scheduled reports/backups are blocked for them. Existing detailed section permissions are preserved, but non-client sections take effect only if private administration is disabled. Review existing full admins: they remain trusted full partners. Suspended accounts must be reactivated explicitly before being granted access. The Telegram ID does **not** create a web-panel login: web sign-in remains separate.

Promoting a group-limited seller with the partner box removes the access restriction but preserves their former group as their personal sales group and retains existing shop details. This saves the ACL and shop atomically, without moving existing clients or changing the owner. Full partners have management authority equivalent to the owner, except that the configured owner's protected administrator entry cannot be altered through admin management.

**Engine and upstream maintenance (v52.2).** Uses sing-box **1.14.4** and Go **1.27.3**, with released alireza0/s-ui **v1.6.6** as the upstream audit baseline. Integrated scope-based protocol lifecycles and hot-reload managers, QUIC session cleanup, idle user-session registry cleanup, retry-safe traffic-stat merging/batch writes, external VLESS XHTTP preservation for Clash/Mihomo, and WireGuard private-key parse errors. Existing local session-kick, quota, traffic-tracker and external-subscription cache/security behavior is retained; safe TLS/Reality boolean handling and AnyTLS defaults were already present. This is a targeted maintenance port, not a wholesale replacement of DrNetwork or every upstream feature. Back up the database and configuration and validate on a staging node before updating production; automated tests do not substitute for a live Telegram/network deployment test.

**Full partners and personal sales groups.** A full administrator is a trusted business partner: they may use global management (including other admins and quotas), while keeping their own shop, prices, card number and sales. In **Telegram bot → Sellers & admins → their own shop**, set **Personal sales group**, then save the page; or use **Edit shop → Personal sales group** in Telegram. Purchases and trials from that shop go into this group, without creating an access scope or hiding other clients. A scoped seller always uses their enforced group, regardless of any desk setting. Invalid personal groups stop delivery rather than falling back to a different group. Empty keeps the existing plan/introducer behavior. Changing a sales group does not move existing clients. **Global shop / All orders / All sales** in the management panel are separate from **My shop**. Full partners retain their own acting Telegram ID in management history. Telegram approval/rejection decisions also record the actual acting admin separately from the seller who owns the sale. Full partners may handle orders regardless of receipt routing or owner-only approval rules; those rules still constrain ordinary sellers and delegated custom approvers. Their personal sales allowance remains enforced for shop delivery, but does not block global client-administration operations. Review existing full administrators before installing: this role now includes administrator and quota management, including its own quota. It is not a security boundary for an untrusted seller. The legacy **Own group for every admin** operation still assigns restrictive access scopes; use the personal sales-group editor instead when full access must remain intact.

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
