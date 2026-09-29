// lumonas-workstation creates encrypted, resumable workstation backups in a
// dedicated LumoNAS share and safely restores them on the client machine.
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/lumonas/lumonas/internal/workstationbackup"
)

type stringListFlag []string

func (values *stringListFlag) String() string { return strings.Join(*values, ",") }
func (values *stringListFlag) Set(value string) error {
	*values = append(*values, value)
	return nil
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	if os.Args[1] == "keygen" {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			fatal(err)
		}
		fmt.Println(base64.StdEncoding.EncodeToString(key))
		return
	}
	var key []byte
	var err error
	if encoded := strings.TrimSpace(os.Getenv("LUMONAS_BACKUP_KEY")); encoded != "" {
		key, err = base64.StdEncoding.DecodeString(encoded)
		if err != nil || len(key) != 32 {
			fatal(errors.New("LUMONAS_BACKUP_KEY must be a 32-byte base64 key from `lumonas-workstation keygen`"))
		}
	}
	config := workstationbackup.Config{
		ServerURL: os.Getenv("LUMONAS_SERVER_URL"),
		Token:     os.Getenv("LUMONAS_API_TOKEN"),
		ShareID:   os.Getenv("LUMONAS_WORKSTATION_SHARE_ID"),
		Key:       key,
		StateDir:  os.Getenv("LUMONAS_BACKUP_STATE_DIR"),
	}
	client, err := workstationbackup.NewClient(config)
	if err != nil {
		fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	switch os.Args[1] {
	case "backup":
		backupFlags := flag.NewFlagSet("backup", flag.ExitOnError)
		source := backupFlags.String("source", "", "directory to back up")
		remotePath := backupFlags.String("path", "/", "existing folder in the selected NAS share")
		var includes, excludes stringListFlag
		backupFlags.Var(&includes, "include", "relative path or glob to include; repeat for multiple selections (all files are included when omitted)")
		backupFlags.Var(&excludes, "exclude", "relative path or glob to omit; repeat to exclude transient or private data")
		_ = backupFlags.Parse(os.Args[2:])
		if *source == "" {
			fatal(errors.New("--source is required"))
		}
		name, err := client.BackupSelected(ctx, *source, *remotePath, workstationbackup.Selection{Include: includes, Exclude: excludes})
		if err != nil {
			fatal(err)
		}
		fmt.Printf("Backup uploaded and verified: %s\n", name)
	case "sync":
		syncFlags := flag.NewFlagSet("sync", flag.ExitOnError)
		source := syncFlags.String("source", "", "local directory to sync to the NAS")
		remotePath := syncFlags.String("path", "/", "folder in the selected NAS share")
		dryRun := syncFlags.Bool("dry-run", false, "show changes without uploading")
		overwrite := syncFlags.Bool("overwrite-existing", false, "replace remote files using compare-and-swap metadata checks")
		var includes, excludes stringListFlag
		syncFlags.Var(&includes, "include", "relative path or glob to include; repeat for multiple selections")
		syncFlags.Var(&excludes, "exclude", "relative path or glob to omit; repeat to exclude transient or private data")
		_ = syncFlags.Parse(os.Args[2:])
		if *source == "" {
			fatal(errors.New("--source is required"))
		}
		report, err := client.SyncPush(ctx, workstationbackup.SyncOptions{Source: *source, RemotePath: *remotePath, Selection: workstationbackup.Selection{Include: includes, Exclude: excludes}, DryRun: *dryRun, Overwrite: *overwrite})
		for _, name := range report.Added {
			fmt.Println("Add:", name)
		}
		for _, name := range report.Updated {
			fmt.Println("Update:", name)
		}
		for _, name := range report.Unchanged {
			fmt.Println("Unchanged:", name)
		}
		for _, name := range report.Conflicts {
			fmt.Println("Conflict:", name)
		}
		if report.SkippedSymlinks > 0 {
			fmt.Printf("Skipped symlinks: %d\n", report.SkippedSymlinks)
		}
		if err != nil {
			fatal(err)
		}
		fmt.Printf("Sync complete: %d added, %d updated, %d unchanged, %d conflicts; %d bytes uploaded\n", len(report.Added), len(report.Updated), len(report.Unchanged), len(report.Conflicts), report.BytesUploaded)
		if len(report.Conflicts) > 0 {
			fatal(errors.New("resolve remote conflicts or rerun with --overwrite-existing after reviewing the target files"))
		}
	case "restore":
		restoreFlags := flag.NewFlagSet("restore", flag.ExitOnError)
		filename := restoreFlags.String("file", "", "backup archive filename in the NAS share")
		remotePath := restoreFlags.String("path", "/", "folder in the selected NAS share")
		destination := restoreFlags.String("to", "", "local restore directory")
		overwrite := restoreFlags.Bool("overwrite", false, "replace existing files")
		_ = restoreFlags.Parse(os.Args[2:])
		if *filename == "" || *destination == "" {
			fatal(errors.New("--file and --to are required"))
		}
		if err := client.Restore(ctx, *filename, *remotePath, *destination, *overwrite); err != nil {
			fatal(err)
		}
		fmt.Printf("Restore completed in %s\n", *destination)
	case "list":
		listFlags := flag.NewFlagSet("list", flag.ExitOnError)
		remotePath := listFlags.String("path", "/", "folder in the selected NAS share")
		_ = listFlags.Parse(os.Args[2:])
		archives, err := client.ListArchives(ctx, *remotePath)
		if err != nil {
			fatal(err)
		}
		if len(archives) == 0 {
			fmt.Println("No workstation backup archives in this folder.")
			return
		}
		for _, archive := range archives {
			fmt.Println(archive)
		}
	default:
		usage()
		os.Exit(2)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "lumonas-workstation:", err)
	os.Exit(1)
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage: lumonas-workstation keygen | backup --source DIR [--path /] [--include GLOB]... [--exclude GLOB]... | sync --source DIR [--path /] [--dry-run] [--overwrite-existing] [--include GLOB]... [--exclude GLOB]... | list [--path /] | restore --file NAME --to DIR [--path /] [--overwrite]")
	fmt.Fprintln(os.Stderr, "Configure LUMONAS_SERVER_URL, LUMONAS_API_TOKEN, and LUMONAS_WORKSTATION_SHARE_ID. Encrypted backup and restore also require LUMONAS_BACKUP_KEY.")
}
