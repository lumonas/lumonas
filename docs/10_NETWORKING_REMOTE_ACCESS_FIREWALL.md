# Networking, Remote Access and Firewall

## Management UI binding

The Debian package and appliance image bind `lumonas-web` to
`0.0.0.0:8081` by default so a browser on the NAS LAN can reach the UI. The
web service uses the locally provisioned TLS certificate, while `lumonasd`
remains on loopback. Set `LUMONAS_WEB_LISTEN` in
`/etc/lumonas/lumonas-web.env` to a specific management interface when the
firewall policy requires narrower exposure. Do not expose the backend port
`8080` directly.

## Backend

Use NetworkManager as the canonical host network manager.

Use its D-Bus API rather than rewriting network files manually.

NetworkManager checkpoints are central to safe remote changes.

## Network overview

Show:

- interfaces;
- link state/speed/duplex;
- IPs;
- gateway;
- DNS;
- Internet reachability;
- Tailscale/WireGuard state;
- current throughput.

## Interface editing

Normal:

- DHCP/static;
- IPv4;
- IPv6 automatic/disabled/static;
- DNS;
- gateway.

Advanced:

- MTU;
- metrics;
- static routes;
- VLAN;
- bond;
- bridge.

## Safe apply

For remote changes:

1. create NetworkManager checkpoint;
2. apply profile;
3. wait for browser/server reconnection;
4. UI confirms “Keep new configuration”;
5. destroy checkpoint/commit.

If acknowledgement does not arrive before timeout, rollback.

This is mandatory for static-IP, route, VLAN, bond and bridge changes.

## VLAN

Wizard:

- parent interface;
- VLAN ID;
- friendly name;
- IP method;
- service exposure.

## Bond/LACP

Modes:

- active-backup;
- 802.3ad LACP;
- selected advanced modes later.

Warn that switch configuration is required for LACP.

## Bridges

Support for advanced Docker/future VM needs.

Do not make bridge creation part of first boot.

## Tailscale

UI:

- install/enable;
- login/auth flow when online;
- assigned IP;
- device name;
- subnet routing/exit-node advanced;
- service binding;
- status/diagnostics.

Core NAS must remain usable without Tailscale.

## WireGuard

Support native WireGuard profiles:

- generate/import keys;
- peer list;
- allowed IPs;
- endpoint;
- QR export where useful;
- route/service binding.

## Firewall

Use nftables.

Normal UI should be service-oriented:

```text
SMB:
LAN allow
Tailscale allow
Other block
```

Do not make users edit nft syntax.

Advanced rule editor can follow later.

## Service binding

Configurable for:

- UI;
- SSH;
- SMB;
- NFS;
- SFTP/FTP;
- rsync;
- reverse proxy.

Binding and firewall should be validated together.

## Diagnostics

Provide:

- ping;
- DNS lookup;
- traceroute;
- port test;
- route table;
- neighbor table;
- DNS state;
- gateway test;
- Internet test;
- LumoNAS update endpoint test;
- Docker registry test.

## Live monitoring

Track:

- bandwidth per interface;
- errors/drops;
- link negotiation changes;
- connectivity history.

Useful alerts:

- link down;
- 1 Gbps unexpectedly negotiated at 100 Mbps;
- high error count;
- gateway unreachable;
- DNS unavailable;
- Internet unavailable.

## Wake-on-LAN

Per interface:

- report WOL support;
- enable/disable;
- show MAC.

The settings toggle is backed by `ethtool` discovery and the typed
`network.wol.set` privileged operation. Unsupported interfaces are shown as
read-only, and interface names are validated before any command is started.

### LAN device discovery and wake-known-hosts

LumoNAS samples the kernel neighbor table (`ip neigh show`) on a background
interval (`LUMONAS_LAN_SCAN_INTERVAL`, default 600s; `0` disables the loop)
and after an on-demand `POST /network/lan/scan`:

- only entries with a valid MAC and a usable neighbor state are recorded;
- the inventory is persisted in SQLite keyed by MAC and interface, so a host
  seen on two segments stays distinct;
- operator-assigned hostnames (`POST /network/lan/hosts/rename`) survive
  every scan; scans only refresh IP and recency;
- `POST /network/lan/hosts/wake` sends a magic packet through the typed
  `network.wol.wake` privileged operation (`etherwake` bound to the named
  interface) with management authorization, a confirmed plan, and an
  operation ID; MAC and interface are validated before any command runs.

## mDNS

Use Avahi to advertise:

- LumoNAS management hostname;
- SMB/Time Machine where appropriate.

Share activation renders a bounded, deterministic Avahi service group and
publishes it through the typed privileged broker. SMB is announced through
`_smb._tcp`; enabled Time Machine shares additionally receive `_adisk._tcp`
disk records. Empty SMB state removes the LumoNAS-managed announcement.

## HTTPS

Support local HTTPS even offline using a LumoNAS local certificate/CA.

Explain that client trust installation may be required.

When a public/private resolvable domain and Internet are available, optionally use ACME.

## Boundaries

Do not become a router/firewall appliance.

No initial:

- DHCP server;
- NAT gateway;
- multi-WAN;
- IDS/IPS;
- traffic shaping.

## Acceptance criteria

- A wrong static IP remotely reverts automatically.
- Interface-name changes after motherboard replacement can be remapped.
- Service binding updates firewall rules coherently.
- Core UI remains usable with Internet unavailable.
