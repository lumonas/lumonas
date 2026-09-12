package recovery

import "testing"

func TestPlanVerifiesBundleContents(t *testing.T) {
	bundle, err := Create(Input{Manifest: Manifest{NASUUID: "nas-1"}, DesiredState: []byte("{}"), Database: []byte("sqlite"), Files: map[string][]byte{"config/shares.json": []byte("[]")}, EncryptedData: []byte("secret")}, []byte("key"))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := Plan(bundle, []byte("key"))
	if err != nil || !plan.Verified || !plan.EncryptedSecrets || !contains(plan.Files, "config/shares.json") {
		t.Fatalf("unexpected restore plan: %#v err=%v", plan, err)
	}
}

func TestCreateRejectsUnsafeFileName(t *testing.T) {
	if _, err := Create(Input{DesiredState: []byte("{}"), Database: []byte("db"), Files: map[string][]byte{"../secrets": []byte("x")}}, []byte("key")); err == nil {
		t.Fatal("expected unsafe file name rejection")
	}
}
