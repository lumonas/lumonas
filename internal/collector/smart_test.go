package collector

import "testing"

func TestReadSMARTParsesHealthAndTemperature(t *testing.T) {
	output := []byte(`{"smart_status":{"passed":true},"temperature":{"current":37},"power_on_time":{"hours":12},"ata_smart_attributes":{"table":[{"name":"Current_Pending_Sector","raw":{"value":2}}]}}`)
	details, err := ReadSMART(func(string, ...string) ([]byte, error) { return output, nil }, "/dev/test")
	if err != nil {
		t.Fatal(err)
	}
	if details.Summary.PendingSectors != 2 || details.TemperatureC == nil || *details.TemperatureC != 37 {
		t.Fatalf("unexpected SMART details %#v", details)
	}
}
