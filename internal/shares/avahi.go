package shares

import (
	"fmt"
	"sort"
	"strings"
)

// RenderAvahi renders the Avahi service announcement for SMB shares. When
// Time Machine shares exist it also publishes the _adisk._tcp records macOS
// uses to discover backup targets (dk0..dkN per share). An empty result
// means no announcement should be published.
func RenderAvahi(values []ManagedShare) (string, error) {
	ordered := append([]ManagedShare(nil), values...)
	sort.Slice(ordered, func(left, right int) bool { return ordered[left].Name < ordered[right].Name })
	hasSMB := false
	disks := make([]string, 0)
	for _, share := range ordered {
		if !share.Enabled {
			continue
		}
		if err := share.Validate(); err != nil {
			return "", err
		}
		for _, protocol := range share.Protocols {
			switch protocol.Name {
			case "smb":
				hasSMB = true
			case "timemachine":
				hasSMB = true
				if validAvahiDiskID(share.ID) {
					disks = append(disks, share.ID)
				}
			}
		}
	}
	if !hasSMB {
		return "", nil
	}
	var builder strings.Builder
	builder.WriteString("<?xml version=\"1.0\" standalone='no'?>\n")
	builder.WriteString("<!DOCTYPE service-group SYSTEM \"avahi-service.dtd\">\n")
	builder.WriteString("<service-group>\n")
	builder.WriteString("  <name replace-wildcards=\"yes\">%h</name>\n")
	builder.WriteString("  <service>\n    <type>_smb._tcp</type>\n    <port>445</port>\n  </service>\n")
	if len(disks) > 0 {
		builder.WriteString("  <service>\n    <type>_adisk._tcp</type>\n    <port>445</port>\n")
		builder.WriteString("    <txt-record>sys=waMa=0,adVF=0x100</txt-record>\n")
		for index, id := range disks {
			fmt.Fprintf(&builder, "    <txt-record>dk%d=%s</txt-record>\n", index, id)
		}
		builder.WriteString("  </service>\n")
	}
	builder.WriteString("</service-group>\n")
	return builder.String(), nil
}

func validAvahiDiskID(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, char := range value {
		valid := char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '.' || char == '_' || char == '-'
		if !valid {
			return false
		}
	}
	return true
}
