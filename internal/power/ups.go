package power

import (
	"context"
	"sort"
	"strconv"
	"strings"

	commandrunner "github.com/lumonas/lumonas/internal/runner"
)

// Runner is injectable so UPS discovery can be tested without a running NUT server.
type Runner func(context.Context, string, ...string) ([]byte, error)

type UPS struct {
	Name          string   `json:"name"`
	Status        string   `json:"status"`
	Manufacturer  string   `json:"manufacturer,omitempty"`
	Model         string   `json:"model,omitempty"`
	Serial        string   `json:"serial,omitempty"`
	ChargePercent *float64 `json:"chargePercent,omitempty"`
	LoadPercent   *float64 `json:"loadPercent,omitempty"`
	RuntimeSec    *float64 `json:"runtimeSec,omitempty"`
	OnBattery     bool     `json:"onBattery"`
}

func Discover(ctx context.Context, names []string, runner Runner) []UPS {
	if runner == nil {
		runner = func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return commandrunner.OutputContext(ctx, name, args...)
		}
	}
	if len(names) == 0 {
		output, err := runner(ctx, "upsc", "-l")
		if err == nil {
			for _, line := range strings.Split(string(output), "\n") {
				if name := strings.TrimSpace(line); name != "" {
					names = append(names, name)
				}
			}
		}
	}
	result := make([]UPS, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		output, err := runner(ctx, "upsc", name)
		if err != nil {
			continue
		}
		unit := parse(name, string(output))
		if unit.Status == "" {
			unit.Status = "unknown"
		}
		unit.OnBattery = strings.Contains(unit.Status, "OB")
		result = append(result, unit)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func parse(name, output string) UPS {
	unit := UPS{Name: name}
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		switch key {
		case "ups.status":
			unit.Status = value
		case "device.mfr":
			unit.Manufacturer = value
		case "device.model":
			unit.Model = value
		case "device.serial":
			unit.Serial = value
		case "battery.charge":
			unit.ChargePercent = number(value)
		case "ups.load":
			unit.LoadPercent = number(value)
		case "battery.runtime":
			unit.RuntimeSec = number(value)
		}
	}
	return unit
}

func number(value string) *float64 {
	value = strings.TrimSpace(strings.TrimSuffix(value, "%"))
	n, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return nil
	}
	return &n
}
