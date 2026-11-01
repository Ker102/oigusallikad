package viga

import (
	"errors"
	"strings"
	"testing"
)

func TestSoovitus(t *testing.T) {
	if s := Soovitus(Uus(EiLeitud, "x", "", "puudub", nil)); s != "" {
		t.Errorf("ei_leitud ei vaja juhist: %q", s)
	}
	for _, err := range []error{Struktuur("x", "", "muutus"), Uus(Blokeeritud, "x", "", "cf", nil), errors.New("paanika")} {
		s := Soovitus(err)
		for _, osa := range []string{"@latest", "/issues", "kontroll", "veateade", "pull request"} {
			if !strings.Contains(s, osa) {
				t.Errorf("%v: juhisest puudub %q", err, osa)
			}
		}
	}
	u := VeateateURL("0.1.0", "Allikas muutunud", "kirjeldus")
	if !strings.HasPrefix(u, Hoidla+"/issues/new?") || !strings.Contains(u, "title=Allikas+muutunud") {
		t.Errorf("link: %s", u)
	}
}
