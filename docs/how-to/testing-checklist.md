# Manual Checklist

## Home

- Start proxy
- Start proxy without CA installed
- Stop proxy

## Filter lists

- Filter lists counters are correct (enabled/total)
- Add custom filter list
- Add custom filter list with invalid URL (error)
- Add custom filter list without name (name becomes same as URL)
- Add trusted custom filter list
- Add filter list with duplicate URL (error)
- Delete custom filter list
- Import filter list
- Import filter list in incorrect format
- Export filter lists
- Enable filter list
- Disable filter list

## My rules

- Add My rule (e.g. `0.0.0.0 www.google.com`)
- Remove My rule
- "Help" button opens guide on How to write rules

## Preferences

- Change language
- Enable "Autostart" (Windows/macOS/Linux)
- Disable "Autostart" (Windows/macOS/Linux)
- Export logs
- Set fixed Port
- Add ignored hosts
- Remove ignored hosts
- Uninstall CA
- Uninstall CA while proxy is running

## Encrypted DNS

- Select a server (e.g. Quad9), start proxy, browse
- Custom server with an invalid URL (error, not saved)
- Add a domain to "Domains that use system DNS"
- A site with a CNAME-cloaked tracker (e.g. tidal.com): the request log shows a blocked request with a CNAME row
- Block the server's addresses in a firewall ("isn't working" status, and a warning bar above every tab: clicking it opens Settings, and a closed bar stays closed until DNS recovers), then unblock (both clear within a minute)
- Connect a VPN that handles all DNS, e.g. Tailscale with MagicDNS (VPN status)
- Sleep and wake: the first page load doesn't stall
- Select "Off" (no status)

## Self‑update (Windows/macOS/Linux)

- Disable updates
- Select "Ask before updating"
- Select "Automatic updates"
