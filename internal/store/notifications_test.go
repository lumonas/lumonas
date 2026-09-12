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
