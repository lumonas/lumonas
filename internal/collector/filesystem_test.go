package collector

import (
	"os"
	"testing"
)

func TestFilesystemUsageReportsCriticalConfiguredPath(t *testing.T) {
	path := os.Getenv("LUMONAS_TEST_FILESYSTEM_PATH")
	if path == "" {
		t.Skip("LUMONAS_TEST_FILESYSTEM_PATH is only set by the disk-full integration smoke")
	}
	values := FilesystemsAt([]string{path})
	if len(values) != 1 {
		t.Fatalf("expected one configured filesystem, got %#v", values)
	}
	if values[0].Path != path || values[0].UsedPercent < 95 || values[0].State != "critical" {
		t.Fatalf("expected critical nearly-full filesystem, got %#v", values[0])
	}
}
