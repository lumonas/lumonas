# File Browser and Data Operations

## Purpose

Provide basic administrative file management without trying to replace a full desktop file manager.

## Supported actions

- browse;
- search within current scope;
- upload;
- download;
- new folder;
- rename;
- copy;
- move;
- delete;
- recycle bin;
- restore;
- archive/extract later;
- properties;
- permission summary.

## Resource-based roots

Normal users browse named storage resources:

```text
Media
Documents
Backups
```

Do not begin at `/`.

Advanced administrators may reveal real paths.

## UI

Desktop:

- breadcrumb;
- table/list;
- optional grid for media;
- name/size/modified/type;
- multi-select;
- drag/drop upload;
- actions.

Mobile uses full-screen list and action sheet.

## Large transfers

Copy/move/upload become global jobs.

Show:

- bytes/files complete;
- throughput;
- current file;
- errors;
- conflict count.

Do not tie transfer lifetime to browser connection.

## Conflict handling

Options:

- overwrite;
- skip;
- rename;
- apply to all.

For destructive overwrite on large operation, summarize before continuing.

## Recycle bin

Per share/pool optional.

Track original path and deletion time.

Retention policy configurable.

Do not claim recycle bin is backup.

## Upload safety

- configurable max chunk;
- resumable upload;
- stream to target/temp location;
- quota/free-space validation;
- filename/path validation;
- no arbitrary path traversal.

## Permissions

Properties show effective simple access.

Do not let file-browser one-off chmod silently conflict with managed share ACL.

If advanced permission editing is supported, route through canonical ACL model.

## Filesystem boundaries

Move across physical branches/pools may become copy+delete.

UI must accurately describe this and use a job.

## SnapRAID awareness

Deleting/moving files is normal filesystem behavior, but Protection UI should reflect unsynced changes afterward.

Do not run SnapRAID sync per file operation.

## Acceptance criteria

- Browser closure does not cancel a server-side file copy.
- Attempting to delete a folder used as Docker appdata warns about dependency.
- User cannot escape allowed roots through symlink/path tricks.
