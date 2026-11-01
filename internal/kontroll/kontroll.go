// Package kontroll käivitab "kanaarilinnu": päringud teadaolevate
// dokumentide vastu, mille sisu on teada. Kui allikas muudab struktuuri,
// ebaõnnestub vähemalt üks kontroll valjult, mitte vaikselt.
package kontroll

import (
	"context"
	"strings"
	"time"

	"github.com/Ker102/oigusallikad/internal/eurlex"
	"github.com/Ker102/oigusallikad/internal/kohtulahendid"
	"github.com/Ker102/oigusallikad/internal/rt"
	"github.com/Ker102/oigusallikad/internal/viga"
)

// Tulemus on ühe kontrolli tulemus.
type Tulemus struct {
	Nimi    string `json:"nimi"`
	Allikas string `json:"allikas"`
	Korras  bool   `json:"korras"`
	Liik    string `json:"vea_liik,omitempty"`
	Teade   string `json:"teade,omitempty"`
	Kestus  string `json:"kestus"`
}

// Aruanne on kõigi kontrollide tulemus.
type Aruanne struct {
	Korras    bool      `json:"korras"`
	Aeg       time.Time `json:"aeg"`
	Tulemused []Tulemus `json:"tulemused"`
}

type kontroll struct {
	nimi, allikas string
	f             func(ctx context.Context) (string, error) // tagastab teksti, milles otsitakse ootust
	ootus         string
}

// Kaivita käivitab kõik kontrollid.
func Kaivita(ctx context.Context, r *rt.RT, k *kohtulahendid.Klient, e *eurlex.Klient) Aruanne {
	kontrollid := []kontroll{
		{"RT seaduste lühendid", "Riigi Teataja", func(ctx context.Context) (string, error) {
			l, err := r.Lyhendid(ctx)
			if err != nil {
				return "", err
			}
			var b strings.Builder
			for _, x := range l {
				b.WriteString(x.Lyhend + " ")
			}
			return b.String(), nil
		}, "THS"},
		{"RT säte: THS § 98 lg 3", "Riigi Teataja", func(ctx context.Context) (string, error) {
			s, err := r.Satted(ctx, "THS", "98", "3", "", "")
			if err != nil {
				return "", err
			}
			return s.Tekst, nil
		}, "teeb käitaja katsed"},
		{"RT ülaindeksiga paragrahv: THS § 88¹ lg 1 p 3", "Riigi Teataja", func(ctx context.Context) (string, error) {
			s, err := r.Satted(ctx, "THS", "88^1", "1", "3", "")
			if err != nil {
				return "", err
			}
			return s.Tekst, nil
		}, "kõrvaldatavate jäätmete koguse"},
		{"RT redaktsioon kuupäeva järgi: JäätS 2023-06-01", "Riigi Teataja", func(ctx context.Context) (string, error) {
			s, err := r.Satted(ctx, "JäätS", "28", "4", "", "2023-06-01")
			if err != nil {
				return "", err
			}
			if s.Akt.KehtivAlates > "2023-06-01" {
				return "", viga.Struktuur("riigiteataja", s.Akt.URL, "valiti redaktsioon, mis algab hiljem kui küsitud kuupäev")
			}
			return s.Tekst, nil
		}, "keskkonnakaitseluba"},
		{"RT akti lisa (PDF): määruse nr 21 lisa 2", "Riigi Teataja", func(ctx context.Context) (string, error) {
			l, err := r.LisaTekst(ctx, "101072026014", "2", "")
			if err != nil {
				return "", err
			}
			return l.Tekst, nil
		}, "Arseen"},
		{"RT kohtulahendite otsing: koldetuhk", "Riigi Teataja", func(ctx context.Context) (string, error) {
			o, err := k.Otsi(ctx, kohtulahendid.Valikud{Tekst: "koldetuhk"})
			if err != nil {
				return "", err
			}
			var b strings.Builder
			for _, t := range o.Tulemused {
				b.WriteString(t.AsjaNr + " ")
			}
			return b.String(), nil
		}, "3-09-1542"},
		{"RT ringkonnakohtu lahend (PDF): 3-15-1620/42 p 24", "Riigi Teataja", func(ctx context.Context) (string, error) {
			t, err := k.Tekst(ctx, kohtulahendid.TekstiValikud{AsjaNr: "3-15-1620/42", Punktid: []int{24}})
			if err != nil {
				return "", err
			}
			return punktideTekst(t), nil
		}, "detailplaneeringus kehtestatud tingimustega"},
		{"Riigikohus (RIKOS): 3-15-443/54 p 13", "Riigikohus", func(ctx context.Context) (string, error) {
			t, err := k.Tekst(ctx, kohtulahendid.TekstiValikud{AsjaNr: "3-15-443/54", Punktid: []int{13}})
			if err != nil {
				return "", err
			}
			return punktideTekst(t), nil
		}, "Ettekirjutuse sisu väljaselgitamisel"},
		{"EUR-Lex kohtuotsus: C-626/22 p 94", "EUR-Lex/Cellar", func(ctx context.Context) (string, error) {
			d, err := e.Dokument(ctx, eurlex.Valikud{Viide: "C-626/22", Punktid: []int{94}})
			if err != nil {
				return "", err
			}
			return osadeTekst(d), nil
		}, "hinnates pidevalt"},
		{"EUR-Lex vana vorming: C-172/08 p 37", "EUR-Lex/Cellar", func(ctx context.Context) (string, error) {
			d, err := e.Dokument(ctx, eurlex.Valikud{Viide: "62008CJ0172", Punktid: []int{37}})
			if err != nil {
				return "", err
			}
			return osadeTekst(d), nil
		}, "kannavad kõik prügila käitamiskulud"},
		// Uuem HTML-vorming (NAME="pointN", NAME="FootnoteN"). Dokument on
		// esialgne tõlge: kui see lõplikuks muutub ja kontroll ebaõnnestub,
		// vaheta see mõne teise uue vormingu dokumendi vastu.
		{"EUR-Lex uus HTML-vorming: C-730/23 ettepanek, allmärkus 49", "EUR-Lex/Cellar", func(ctx context.Context) (string, error) {
			d, err := e.Dokument(ctx, eurlex.Valikud{Viide: "C-730/23", Liik: "ettepanek", Allmarkused: []int{49}})
			if err != nil {
				return "", err
			}
			return osadeTekst(d), nil
		}, "2008/98"},
		{"EUR-Lex direktiiv: 2008/98/EÜ art 15", "EUR-Lex/Cellar", func(ctx context.Context) (string, error) {
			d, err := e.Dokument(ctx, eurlex.Valikud{Viide: "32008L0098", Artiklid: []string{"15"}})
			if err != nil {
				return "", err
			}
			return osadeTekst(d), nil
		}, "eeltöötlemiseks"},
	}
	ar := Aruanne{Korras: true, Aeg: time.Now().UTC()}
	for _, kk := range kontrollid {
		algus := time.Now()
		cctx, cancel := context.WithTimeout(ctx, 90*time.Second)
		t, err := kk.f(cctx)
		cancel()
		res := Tulemus{Nimi: kk.nimi, Allikas: kk.allikas, Kestus: time.Since(algus).Round(time.Millisecond).String()}
		switch {
		case err != nil:
			res.Liik = string(viga.LiikVeast(err))
			if res.Liik == "" {
				res.Liik = "muu"
			}
			res.Teade = err.Error()
		case !strings.Contains(t, kk.ootus):
			res.Liik = string(viga.StruktuurMuutunud)
			res.Teade = "vastus saadi, kuid oodatud tekst puudub: " + kk.ootus
		default:
			res.Korras = true
		}
		if !res.Korras {
			ar.Korras = false
		}
		ar.Tulemused = append(ar.Tulemused, res)
	}
	return ar
}

func punktideTekst(t *kohtulahendid.TekstiVastus) string {
	var b strings.Builder
	for _, p := range t.Punktid {
		b.WriteString(p.Tekst + "\n")
	}
	return b.String()
}

func osadeTekst(d *eurlex.Vastus) string {
	var b strings.Builder
	for _, o := range d.Osad {
		b.WriteString(o.Tekst + "\n")
	}
	return b.String()
}
