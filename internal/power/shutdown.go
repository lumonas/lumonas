package power

import (
	"context"
	"errors"
)

type ShutdownStep struct {
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
}

type ShutdownRunner func(context.Context, string, ...string) ([]byte, error)

func OrderedShutdown(action string) ([]ShutdownStep, error) {
	if action != "poweroff" && action != "reboot" {
		return nil, errors.New("shutdown action must be poweroff or reboot")
	}
	return []ShutdownStep{
		{Name: "stop-jobs", Command: "systemctl", Args: []string{"stop", "mynas-jobs.target"}},
		{Name: "stop-services", Command: "systemctl", Args: []string{"stop", "mynas-services.target"}},
		{Name: "flush-writes", Command: "sync"},
		{Name: "unmount-storage", Command: "systemctl", Args: []string{"stop", "mynas-storage.target"}},
		{Name: "power-action", Command: "systemctl", Args: []string{action}},
	}, nil
}

func ExecuteShutdown(ctx context.Context, action string, runner ShutdownRunner) error {
	steps, err := OrderedShutdown(action)
	if err != nil {
		return err
	}
	if runner == nil {
		return errors.New("shutdown runner is required")
	}
	for _, step := range steps {
		if _, err := runner(ctx, step.Command, step.Args...); err != nil {
			return errors.New("shutdown step " + step.Name + " failed")
		}
	}
	return nil
}

type ShutdownPolicy struct {
	Enabled           bool
	MinimumRuntimeSec float64
	MinimumCharge     float64
}

func ShouldShutdown(unit UPS, policy ShutdownPolicy) bool {
	if !policy.Enabled || !unit.OnBattery {
		return false
	}
	if unit.RuntimeSec != nil && *unit.RuntimeSec <= policy.MinimumRuntimeSec {
		return true
	}
	return unit.ChargePercent != nil && *unit.ChargePercent <= policy.MinimumCharge
}
