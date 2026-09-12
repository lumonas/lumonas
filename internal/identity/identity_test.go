package identity

import "testing"

func TestValidateCreateSeparatesManagementAndFileUsers(t *testing.T) {
	if err := ValidateCreate(CreateInput{Kind: KindUser, Name: "media", ManagementRole: RoleNone}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateCreate(CreateInput{Kind: KindUser, Name: "operator", Password: "a-long-development-password", ManagementRole: RoleOperator}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateCreate(CreateInput{Kind: KindService, Name: "backup", ManagementRole: RoleAdmin}); err == nil {
		t.Fatal("service identity should not receive management access")
	}
}

func TestValidateCreateRejectsUnsafeNamesAndPasswords(t *testing.T) {
	for _, input := range []CreateInput{
		{Kind: KindUser, Name: "../escape"},
		{Kind: KindGroup, Name: "family team"},
		{Kind: KindUser, Name: "admin", ManagementRole: RoleAdmin, Password: "short"},
	} {
		if err := ValidateCreate(input); err == nil {
			t.Fatalf("expected validation failure for %#v", input)
		}
	}
}
