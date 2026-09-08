package set

import (
	"strings"
	"testing"
)

func TestRestoreWithoutOwnershipDoesNotChangeProfilePolicies(t *testing.T) {
	journal := perehvat(t, otvetProfiley(vyvodProfilyaRu))
	if err := VyklyuchitVesTrafik(); err != nil {
		t.Fatal(err)
	}
	for _, call := range *journal {
		if strings.Contains(strings.Join(call.argumenty, " "), " set ") {
			t.Fatalf("changed foreign profile policy without rollback: %v", call.argumenty)
		}
	}
}

func TestRestorePreservesEachProfilePolicy(t *testing.T) {
	journal := perehvat(t, otvetProfiley(vyvodProfilyaRu))
	profiles := []ProfilDo{
		{Imya: "domain", Vklyuchen: true, Politika: "BlockInbound,BlockOutbound"},
		{Imya: "private", Vklyuchen: true, Politika: "AllowInbound,AllowOutbound"},
		{Imya: "public", Vklyuchen: false, Politika: "BlockInbound,AllowOutbound"},
	}
	if err := ZapisatOtkat(Otkat{Profili: profiles}); err != nil {
		t.Fatal(err)
	}
	if err := VyklyuchitVesTrafik(); err != nil {
		t.Fatal(err)
	}
	for _, profile := range profiles {
		wanted := "advfirewall set " + profile.Imya + "profile firewallpolicy " + strings.ToLower(profile.Politika)
		found := false
		for _, call := range *journal {
			if strings.Join(call.argumenty, " ") == wanted {
				found = true
			}
		}
		if !found {
			t.Errorf("did not restore %s", wanted)
		}
	}
}
