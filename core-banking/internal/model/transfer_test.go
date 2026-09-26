package model

import (
	"encoding/json"
	"testing"
)

func TestMoney(t *testing.T) {
	for _, s := range []string{"0", "100000", "0.01", "99999999999999999.99", "-1.25"} {
		var m Money
		if err := json.Unmarshal([]byte(s), &m); err != nil {
			t.Fatal(s, err)
		}
		b, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		var again Money
		if err = json.Unmarshal(b, &again); err != nil || again != m {
			t.Fatal(string(b), err)
		}
	}
	for _, s := range []string{`"1"`, "null", "1e3", "1.001", "100000000000000000.00"} {
		var m Money
		if json.Unmarshal([]byte(s), &m) == nil {
			t.Fatalf("accepted %s", s)
		}
	}
	a, _ := ParseMoney("99999999999999999.98")
	b, err := a.Add("0.01")
	if err != nil || b != "99999999999999999.99" {
		t.Fatal(b, err)
	}
	if _, err = b.Add("0.01"); err == nil {
		t.Fatal("overflow accepted")
	}
}
