# Networking, Remote Access and Firewall

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

Future: LAN device list + wake known hosts.

## mDNS

Use Avahi to advertise:

- LumoNAS management hostname;
- SMB/Time Machine where appropriate.

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
