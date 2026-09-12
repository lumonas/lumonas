# Future Product Features

These features fit the product philosophy but can evolve after the primary architecture is stable.

## Storage

- dual/multi-parity SnapRAID UI where appropriate;
- ZFS module;
- Btrfs module;
- read-only snapshot browsing;
- USB backup rotation sets;
- disk bay mapping and enclosure visualization;
- HBA locate LED;
- SMART predictive trend rules;
- automatic cold-data placement suggestions.

## Backup

- MyNAS-to-MyNAS replication protocol;
- remote deduplicated repository;
- immutable backup target mode;
- ransomware-resistant retention;
- snapshot-aware application backup;
- backup bandwidth scheduling;
- periodic restore drills.

## Docker

- community catalog repositories;
- dependency diagrams;
- safe app update channels;
- canary app update;
- registry mirror;
- local OCI registry;
- compose profiles UI;
- Docker network topology visualization;
- app permission/security profile;
- GPU allocation wizard.

## Networking

- richer VLAN topology visualization;
- NIC bonding diagnostics;
- network throughput tests between MyNAS nodes;
- SMB multichannel assistant;
- 10GbE tuning recommendations;
- mDNS service explorer.

## Storage services

- WebDAV;
- S3-compatible local object storage through a packaged app/integration;
- SMB shadow-copy integration if snapshot backend exists;
- advanced rsync module permissions.

## Security

- passkey-first admin;
- hardware security key;
- role-based administration;
- just-in-time destructive unlock;
- IP/network admin restrictions;
- local IDS-style admin-login anomaly notifications.

## Hardware

- fan-control integration where hardware supports safe controls;
- IPMI monitoring;
- HBA firmware inventory;
- ECC status;
- PSU/UPS efficiency history;
- SMART/controller topology map.

## Multi-NAS

- dashboard for multiple MyNAS machines;
- centralized notifications;
- remote backup pairing;
- config comparison;
- shared app catalog mirror.

## Search

- global indexed file metadata search;
- optional media/document indexing as an app rather than mandatory core.

## API and automation

- scoped API tokens;
- Home Assistant integration;
- event-triggered automations;
- webhook-triggered jobs;
- CLI client built on public API.

## Installer

- browser-first network installer;
- automated PXE installer;
- USB auto-recovery mode;
- ARM64 image if supported.

## Guiding rule

Future features should remain modular. If a feature can sensibly run as a Docker app without privileged host integration, prefer an app over permanently expanding the root-level NAS core.
