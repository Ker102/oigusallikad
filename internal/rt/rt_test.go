package rt

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Ker102/oigusallikad/internal/klient"
	"github.com/Ker102/oigusallikad/internal/viga"
)

const naidisXML = `<?xml version="1.0" encoding="UTF-8"?><oigusakt xmlns="Juurakt"><sisu>
<peatykk><peatykkNr>4</peatykkNr><kuvatavNr><![CDATA[4. peatükk]]></kuvatavNr><peatykkPealkiri>Jäätmepõletus</peatykkPealkiri>
<paragrahv id="para88b1"><paragrahvNr ylaIndeks="1">88</paragrahvNr><kuvatavNr><![CDATA[§ 88<sup>1</sup>.]]></kuvatavNr><paragrahvPealkiri>Loa taotlus</paragrahvPealkiri>
 <loige><loigeNr>1</loigeNr><kuvatavNr><![CDATA[(1)]]></kuvatavNr><sisuTekst><tavatekst>Käitaja esitab:</tavatekst></sisuTekst>
  <alampunkt><alampunktNr>3</alampunktNr><kuvatavNr><![CDATA[3)]]></kuvatavNr><sisuTekst><tavatekst>andmed jäätmete koguse kohta.</tavatekst></sisuTekst></alampunkt>
 </loige>
 <muutmismarge><avaldamismarge><RTosa>RT I</RTosa><avaldamineKuupaev>2019-12-21</avaldamineKuupaev><RTartikkel>1</RTartikkel></avaldamismarge><joustumine>2020-01-01</joustumine></muutmismarge>
</paragrahv>
<paragrahv id="para98"><paragrahvNr>98</paragrahvNr><kuvatavNr><![CDATA[§ 98.]]></kuvatavNr><paragrahvPealkiri>Jäägid</paragrahvPealkiri>
 <loige><loigeNr>1</loigeNr><kuvatavNr><![CDATA[(1)]]></kuvatavNr><sisuTekst><tavatekst>Jääkide kogust vähendatakse.</tavatekst></sisuTekst></loige>
 <loige><loigeNr>3</loigeNr><kuvatavNr><![CDATA[(3)]]></kuvatavNr><sisuTekst><tavatekst>Enne jääkide kõrvaldamist teeb käitaja katsed (§ 88<sup>1</sup>).</tavatekst></sisuTekst></loige>
</paragrahv></peatykk></sisu></oigusakt>`

func testServer(t *testing.T, lyhendeid int, xml string) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/akt/lyhendid", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var b strings.Builder
		b.WriteString("[")
		for i := 0; i < lyhendeid; i++ {
			if i > 0 {
				b.WriteString(",")
			}
			l, p := fmt.Sprintf("X%d", i), fmt.Sprintf("Seadus %d", i)
			id := 900 + i
			if i == 0 {
				l, p, id = "THS", "Tööstusheite seadus", 300
			}
			fmt.Fprintf(&b, `{"id":%d,"lyhend":%q,"pealkiri":%q}`, id, l, p)
		}
		b.WriteString("]")
		w.Write([]byte(b.String()))
	})
	meta := func(id int, algus, lopp string) string {
		return fmt.Sprintf(`{"grupiId":1000,"tekstiliik":"terviktekst","dokumentliik":"seadus","aktiParameetrid":{"pealkiri":"Tööstusheite seadus","lyhend":"THS","kehtivuseAlgus":%q,"kehtivuseLopp":%q},"id_":%d}`, algus, lopp, id)
	}
	mux.HandleFunc("/akt/300", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(meta(300, "2026-09-30T21:00:00Z", "2026-12-30T22:00:00Z")))
	})
	mux.HandleFunc("/akt/200", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(meta(200, "2023-04-30T21:00:00Z", "2023-06-29T21:00:00Z")))
	})
	mux.HandleFunc("/akt/1000/redaktsioonid", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"redaktsioonid":[{"id":300,"kehtivuseAlgus":"2026-09-30T21:00:00Z"},{"id":250,"kehtivuseAlgus":"2023-06-30T21:00:00Z"},{"id":200,"kehtivuseAlgus":"2023-04-30T21:00:00Z"}]}`))
	})
	for _, id := range []string{"300", "200"} {
		mux.HandleFunc("/akt/"+id+"/blob-xml", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(xml))
		})
	}
	return httptest.NewServer(mux)
}

func uusRT(t *testing.T, srv *httptest.Server) *RT {
	baas = srv.URL
	return Uus(klient.Uus(klient.Valikud{Vahemalu: "-", Vahe: time.Millisecond, IlmaKordusteta: true}))
}

func TestSatted(t *testing.T) {
	srv := testServer(t, 150, naidisXML)
	defer srv.Close()
	r := uusRT(t, srv)
	ctx := context.Background()

	v, err := r.Satted(ctx, "ths", "98", "3", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(v.Tekst, "(3) Enne jääkide kõrvaldamist") || !strings.Contains(v.Tekst, "§ 88¹") {
		t.Errorf("vale tekst: %q", v.Tekst)
	}
	if v.Viide != "THS § 98 lg 3" || v.Akt.ID != 300 {
		t.Errorf("viide/akt: %s %d", v.Viide, v.Akt.ID)
	}

	v, err = r.Satted(ctx, "THS", "88¹", "1", "3", "")
	if err != nil {
		t.Fatal(err)
	}
	if v.Tekst != "3) andmed jäätmete koguse kohta." || len(v.Muudetud) != 1 || v.Muudetud[0] != "[RT I, 21.12.2019, 1 - jõust. 01.01.2020]" {
		t.Errorf("ülaindeks/punkt: %q %v", v.Tekst, v.Muudetud)
	}

	// kuupäeva järgi valitakse redaktsioon 200 (kehtiv 2023-05-01 – 2023-06-30)
	v, err = r.Satted(ctx, "THS", "98", "", "", "2023-06-01")
	if err != nil {
		t.Fatal(err)
	}
	if v.Akt.ID != 200 || v.Akt.KehtivAlates != "2023-05-01" {
		t.Errorf("redaktsiooni valik: id %d alates %s", v.Akt.ID, v.Akt.KehtivAlates)
	}

	if _, err := r.Satted(ctx, "THS", "999", "", "", ""); !viga.LiikOn(err, viga.EiLeitud) {
		t.Errorf("olematu paragrahv: %v", err)
	}
	if _, err := r.Satted(ctx, "THS", "98", "", "", "2001-01-01"); !viga.LiikOn(err, viga.EiLeitud) {
		t.Errorf("enne esimest redaktsiooni: %v", err)
	}
}

func TestStruktuuriMuutus(t *testing.T) {
	// lühendite loend liiga lühike -> struktuur muutunud, mitte "ei leitud"
	srv := testServer(t, 5, naidisXML)
	r := uusRT(t, srv)
	if _, err := r.Satted(context.Background(), "THS", "98", "", "", ""); !viga.LiikOn(err, viga.StruktuurMuutunud) {
		t.Errorf("lühike lühendite loend: %v", err)
	}
	srv.Close()

	// XML ilma paragrahvideta -> struktuur muutunud
	srv = testServer(t, 150, `<?xml version="1.0"?><oigusakt><sisu><uusElement/></sisu></oigusakt>`)
	defer srv.Close()
	r = uusRT(t, srv)
	if _, err := r.Satted(context.Background(), "THS", "98", "", "", ""); !viga.LiikOn(err, viga.StruktuurMuutunud) {
		t.Errorf("XML ilma paragrahvideta: %v", err)
	}
}

func TestParsiNumber(t *testing.T) {
	cases := map[string][2]string{"98": {"98", ""}, "88^1": {"88", "1"}, "88¹": {"88", "1"}, "§ 88 prim 1": {"88", "1"}, "12-2": {"12", "2"}}
	for in, want := range cases {
		n, y, ok := ParsiNumber(in)
		if !ok || n != want[0] || y != want[1] {
			t.Errorf("ParsiNumber(%q) = %q %q %v", in, n, y, ok)
		}
	}
	if _, _, ok := ParsiNumber("abc"); ok {
		t.Error("abc ei tohi sobida")
	}
}

func TestSisukordJaOtsing(t *testing.T) {
	srv := testServer(t, 150, naidisXML)
	defer srv.Close()
	r := uusRT(t, srv)
	s, err := r.AktiSisukord(context.Background(), "THS", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Read) != 3 || s.Read[1].Nr != "§ 88¹." {
		t.Errorf("sisukord: %+v", s.Read)
	}
	o, err := r.OtsiAktist(context.Background(), "THS", "käitaja katse", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(o.Vasted) != 1 || o.Vasted[0].Viide != "§ 98 lg 3" {
		t.Errorf("otsing: %+v", o.Vasted)
	}
}
