package tekst

import (
	"strings"
	"testing"
)

func TestJaotaPunktideks(t *testing.T) {
	lahend := `3-15-443/54
RIIGIKOHUS
RESOLUTSIOON
1. Rahuldada kassatsioonkaebus osaliselt.
2. Tühistada ringkonnakohtu otsus.
ASJAOLUD
1.
Veterinaarkeskus tegi järelevalvetoiminguid.
2.
Kaebaja esitas kaebuse.
8(10)
3-15-443
3. Halduskohus jättis kaebuse rahuldamata.
Kolleegium leiab, et punktis 1. kirjeldatu ei muuda midagi.
4. Kolleegium nõustub.
5. Ettekirjutuse sisu väljaselgitamisel võib arvestada põhjendust.
`
	p := JaotaPunktideks(lahend)
	if len(p) != 5 {
		t.Fatalf("ootasin 5 põhjenduste punkti, sain %d: %+v", len(p), p)
	}
	if !strings.HasPrefix(p[0].Tekst, "Veterinaarkeskus") {
		t.Errorf("1. punkt peab olema põhjendustest, mitte resolutsioonist: %q", p[0].Tekst)
	}
	if strings.Contains(p[1].Tekst, "8(10)") || strings.Contains(p[1].Tekst, "3-15-443") {
		t.Errorf("lehe jalus jäi teksti: %q", p[1].Tekst)
	}
	if !strings.Contains(p[2].Tekst, "punktis 1. kirjeldatu") {
		t.Errorf("3. punkt: %q", p[2].Tekst)
	}
	valitud, puudu := ValiPunktid(p, []int{5, 9})
	if len(valitud) != 1 || len(puudu) != 1 || puudu[0] != 9 {
		t.Errorf("ValiPunktid: %v %v", valitud, puudu)
	}
}

func TestParsiPunktid(t *testing.T) {
	got, err := ParsiPunktid("35-37, 41 12")
	if err != nil {
		t.Fatal(err)
	}
	want := []int{12, 35, 36, 37, 41}
	if len(got) != len(want) {
		t.Fatalf("%v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%v", got)
		}
	}
	if _, err := ParsiPunktid("a-b"); err == nil {
		t.Error("ootasin viga")
	}
}

func TestHTMLTekstiks(t *testing.T) {
	s, err := HTMLTekstiks([]byte(`<html><head><script>x()</script></head><body><p>Esimene&nbsp;lõik</p><div>Teine <b>lõik</b></div></body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	if s != "Esimene lõik\n\nTeine lõik" {
		t.Errorf("%q", s)
	}
}
