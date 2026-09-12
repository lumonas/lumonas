package network

import "testing"

func TestInterfacesAreSortedAndValid(t *testing.T) {
	interfaces, err := Interfaces()
	if err != nil {
		t.Fatal(err)
	}
	for index := 1; index < len(interfaces); index++ {
		if interfaces[index-1].Name > interfaces[index].Name {
			t.Fatal("interfaces are not sorted")
		}
	}
}
