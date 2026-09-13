# Real-client interoperability tests

These scripts exercise a **running LumoNAS appliance** with real protocol
clients. They exist so the "not fully proven" interoperability matrix
(SMB, NFS, SFTP, FTP/FTPS, rsync, Time Machine/Avahi) becomes executable on
hardware instead of a manual checklist.

Every check follows the same contract:

- a missing client tool **skips**;
- an unreachable service **skips**;
- a protocol-level roundtrip failure **fails** (non-zero exit);
- `run-all.sh` exits non-zero only when at least one check failed, so partial
  environments still produce a useful pass/skip/fail summary.

## Running

On a machine that can reach the appliance (the appliance itself, or any
workstation with the client tools installed):

```sh
export LUMONAS_INTEROP_HOST=192.168.1.50
export LUMONAS_INTEROP_SHARE=media
export LUMONAS_INTEROP_USER=fileuser          # a LumoNAS file user
export LUMONAS_INTEROP_PASSWORD='...'
bash scripts/interop/run-all.sh
```

Individual checks can be run directly, e.g. `bash scripts/interop/interop-smb.sh`.

## Check matrix

| Script | Client tool | Service port | Notes |
|---|---|---|---|
| `interop-smb.sh` | `smbclient` (SMB3) | 445 | list + put + get roundtrip + delete |
| `interop-nfs.sh` | `showmount`, `mount -t nfs4` | 2049 | root on Linux required |
| `interop-sftp.sh` | `sftp` | 22 | SSH key auth recommended |
| `interop-ftp.sh` | `curl` | 21 | FTP and FTPS (`--ssl-reqd`) |
| `interop-rsync.sh` | `rsync` | 873 or 22 | module mode (rsync daemon) or ssh mode |
| `interop-timemachine.sh` | `avahi-browse`, `tmutil` | mDNS | advertisement discovery; `tmutil` on macOS |

## Extra environment variables

| Variable | Default | Used by |
|---|---|---|
| `LUMONAS_INTEROP_SFTP_PORT` | `22` | sftp |
| `LUMONAS_INTEROP_FTP_PORT` | `21` | ftp/ftps |
| `LUMONAS_INTEROP_RSYNC_MODE` | `module` | rsync (`module` or `ssh`) |
| `LUMONAS_INTEROP_RSYNC_PORT` | `873` | rsync module mode |
| `LUMONAS_INTEROP_RSYNC_MODULE` | share name | rsync module mode |
| `LUMONAS_INTEROP_REQUIRE_ADVERTISEMENTS` | `false` | Time Machine/Avahi; fail when SMB mDNS advertisement is absent |
| `LUMONAS_INTEROP_WORKDIR` | `mktemp -d` | scratch space |

## Hardware acceptance context

These scripts complement `docs/23_ACCEPTANCE_TESTS.md`: the acceptance
document defines what must be verified before Stable, and this directory makes
the client-interop rows executable wherever the matching hardware and clients
exist (CI runners cannot provide SMB/NFS clients, so they are intentionally
absent from the release gates and belong to hardware acceptance runs).

For ordinary workstation checks, missing mDNS advertisements are skipped just
like other unavailable services. Hardware acceptance runs should set
`LUMONAS_INTEROP_REQUIRE_ADVERTISEMENTS=true` so an appliance that is not
advertising SMB is reported as a failure.
