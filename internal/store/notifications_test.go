package store

import (
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/notify"
)

func TestNotificationChannelCredentialsPersistEncryptedAndRedacted(t *testing.T) {
	database, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	value, err := database.SaveNotificationChannel(notify.Channel{ID: "ch-1", Type: "telegram", Label: "Ops", Target: "telegram://ops", Enabled: true}, notify.Credentials{Token: "secret"}, []byte("recovery-key"))
	if err != nil || !value.Configured {
		t.Fatalf("save failed: %#v %v", value, err)
	}
	listed, err := database.ListNotificationChannels()
	if err != nil || len(listed) != 1 || listed[0].Configured != true {
		t.Fatalf("unexpected list: %#v %v", listed, err)
	}
	_, credentials, err := database.NotificationChannel("ch-1", []byte("recovery-key"))
	if err != nil || credentials.Token != "secret" {
		t.Fatalf("credentials did not round trip: %#v %v", credentials, err)
	}
	if _, _, err := database.NotificationChannel("ch-1", []byte("wrong")); err == nil {
		t.Fatal("wrong decryption key was accepted")
	}
}

func TestNotificationDeliveryStatusPersistsWithoutSecrets(t *testing.T) {
	database, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.SaveNotificationDelivery(notify.Delivery{ID: "delivery-1", ChannelID: "ch-1", EventType: "disk.smart.warning", State: "failed", AttemptedAt: time.Now().UTC(), Error: "provider returned HTTP 503"}); err != nil {
		t.Fatal(err)
	}
	values, err := database.NotificationDeliveries(10)
	if err != nil || len(values) != 1 || values[0].State != "failed" || values[0].Error == "" {
		t.Fatalf("unexpected delivery status: %#v %v", values, err)
	}
}

func TestNotificationFailureWindowPersistsAndClears(t *testing.T) {
	database, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	suppressedUntil := time.Now().UTC().Add(5 * time.Minute)
	if err := database.SaveNotificationFailure("ch-1", "disk.smart.warning", 3, suppressedUntil); err != nil {
		t.Fatal(err)
	}
	failures, actualUntil, found, err := database.NotificationFailure("ch-1", "disk.smart.warning")
	if err != nil || !found || failures != 3 || actualUntil.IsZero() {
		t.Fatalf("failure window did not round trip: failures=%d until=%v found=%v err=%v", failures, actualUntil, found, err)
	}
	if err := database.ClearNotificationFailure("ch-1", "disk.smart.warning"); err != nil {
		t.Fatal(err)
	}
	if _, _, found, err := database.NotificationFailure("ch-1", "disk.smart.warning"); err != nil || found {
		t.Fatalf("failure window was not cleared: found=%v err=%v", found, err)
	}
}
