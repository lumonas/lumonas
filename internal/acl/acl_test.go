package acl

import "testing"

func TestValidateACLRequest(t *testing.T) {
	if err := Validate("/srv/pools/media", []Entry{{Principal: "family", Level: "read"}}, true); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/etc/passwd", "/srv/pools/../disks"} {
		if err := Validate(path, []Entry{{Principal: "family", Level: "write"}}, false); err == nil {
			t.Fatalf("unsafe path %q was accepted", path)
		}
	}
	if err := Validate("/srv/pools/media", []Entry{{Principal: "family", Level: "read"}, {Principal: "family", Level: "write"}}, false); err == nil {
		t.Fatal("duplicate principal was accepted")
	}
}
