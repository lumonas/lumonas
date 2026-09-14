package main

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// debianUpdatesStatus counts pending package upgrades from the APT cache.
// It is read-only: `apt-get -s dist-upgrade` simulates against the existing
// package lists and never touches the system.
func debianUpdatesStatus(run command) response {
	out, err := run("apt-get", "-s", "dist-upgrade")
	if err != nil {
		return response{Error: "apt simulation failed"}
	}
	pending, security := countAptInstalls(string(out))
	return response{OK: true, Data: map[string]any{"pendingCount": pending, "securityCount": security}}
}

var aptInstPattern = regexp.MustCompile(`^Inst\s+(\S+)(\s+\[[^\]]*\])?\s+\((\S+)(\s+=>\s+(\S+))?`)

func countAptInstalls(simulation string) (pending, security int) {
	for _, line := range strings.Split(simulation, "\n") {
		match := aptInstPattern.FindStringSubmatch(strings.TrimSpace(line))
		if match == nil {
			continue
		}
		pending++
		// Debian security updates arrive via the security suite; the
		// simulation does not carry suite metadata, so fall back to the
		// common security-origin markers in version or section suffixes.
		if isSecurityUpdate(line) {
			security++
		}
	}
	return pending, security
}

func isSecurityUpdate(instLine string) bool {
	lower := strings.ToLower(instLine)
	return strings.Contains(lower, "security") || strings.Contains(lower, "-security")
}

// dockerLogUsage reports the on-disk size of the largest container JSON logs.
// /var/lib/docker is root-only, so the privileged worker performs the scan.
// logConsumer identifies a container and the size of its JSON log. The
// field names match the settings UI contract (topConsumers).
type logConsumer struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"sizeBytes"`
}

func dockerLogUsage(run command) response {
	out, err := run("sh", "-c", "du -sb /var/lib/docker/containers/*/*-json.log 2>/dev/null || true")
	if err != nil {
		return response{Error: "docker log usage scan failed"}
	}
	entries := make([]logConsumer, 0)
	var total int64
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) != 2 {
			continue
		}
		size, parseErr := strconv.ParseInt(fields[0], 10, 64)
		if parseErr != nil || size <= 0 {
			continue
		}
		container := fields[1]
		if marker := strings.LastIndexByte(container, '/'); marker >= 0 {
			container = container[:marker]
		}
		if marker := strings.LastIndexByte(container, '/'); marker >= 0 {
			container = container[marker+1:]
		}
		entries = append(entries, logConsumer{Name: container, SizeBytes: size})
		total += size
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].SizeBytes > entries[j].SizeBytes })
	if len(entries) > 5 {
		entries = entries[:5]
	}
	if entries == nil {
		entries = []logConsumer{}
	}
	return response{OK: true, Data: map[string]any{"topConsumers": entries, "totalBytes": total}}
}
