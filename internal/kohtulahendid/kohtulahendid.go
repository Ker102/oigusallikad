// Package kohtulahendid otsib Eesti kohtulahendeid Riigi Teataja avalikust
// API-st ja laeb nende teksti: Riigikohtu lahendid Riigikohtu lahendite
// andmebaasist (rikos.rik.ee, HTML), muud lahendid Riigi Teatajast (PDF).
package kohtulahendid

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/Ker102/oigusallikad/internal/klient"
	"github.com/Ker102/oigusallikad/internal/tekst"
	"github.com/Ker102/oigusallikad/internal/viga"
)

const (
	allikasRT = "riigiteataja-kohtulahendid"
	allikasRK = "riigikohus"
)

// Aadressid on muutujad, et teste saaks käivitada kohaliku serveri vastu.
var (
	baas        = "https://www.riigiteataja.ee/public-api/api/v1"
	rikos       = "https://rikos.rik.ee/"
	detailidURL = "https://www.riigiteataja.ee/kohtulahendid/detailid.html?id="
)

var tallinn, _ = time.LoadLocation("Europe/Tallinn")

// Klient on kohtulahendite klient.
type Klient struct{ k *klient.Klient }

// Uus loob kliendi.
func Uus(k *klient.Klient) *Klient { return &Klient{k: k} }

// Lahend on otsingutulemuse kirje.
type Lahend struct {
	AsjaNr      string   `json:"asja_nr"`
	Kohus       string   `json:"kohus"`
	Kuupaev     string   `json:"kuupaev,omitempty"`
	Menetlus    string   `json:"menetlus,omitempty"`
	Staatus     string   `json:"staatus,omitempty"`
	Kategooriad []string `json:"kategooriad,omitempty"`
	URL         string   `json:"url"`
	objektID    int64
	failiID     int64
}

// OtsinguVastus on otsingu tulemus.
type OtsinguVastus struct {
	Paring    string   `json:"paring"`
	Kokku     int      `json:"kokku"`
	Tulemused []Lahend `json:"tulemused"`
	Markus    string   `json:"markus,omitempty"`
	AndmeURL  string   `json:"andme_url"`
}

// Otsingu valikud.
type Valikud struct {
	Tekst  string // täistekstiotsing
	AsjaNr string // kohtuasja number (pealkirjaotsing)
	// Aste: "riigikohus", "ringkonnakohus", "esimene" (maa- ja halduskohtud) või tühi.
	Aste         string
	Alates, Kuni string // AAAA-KK-PP, filtreeritakse tulemuste peal
	Piir         int
}

type tulemusJSON struct {
	Kokku     *int `json:"kokku"`
	Tulemused []struct {
		AsjaNr   string `json:"kohtuasjaNumber"`
		Kohus    string `json:"toiminguEsitajaAsutus"`
		Aeg      string `json:"lahendiKuulutamiseAeg"`
		Menetlus string `json:"menetlusLiikKlVaartus"`
		Staatus  string `json:"staatusKlVaartus"`
		ObjektID int64  `json:"objektId"`
		FailiID  int64  `json:"avalikustatudFailiId"`
		Kat      []struct {
			Kat  string `json:"asjaKategooriaVaartus"`
			Alam string `json:"asjaAlakategooriaVaartus"`
		} `json:"asjaKategooriaList"`
	} `json:"tulemused"`
}

func otsinguKeha(tekst string, sisus, morf bool) []byte {
	b := map[string]any{
		"general": map[string]any{
			"searchInText": sisus, "searchInTitle": !sisus, "searchText": tekst, "searchText2": "",
			"logicalOperator": "AND", "morphSearch": morf, "searchAfter": 0, "sort": "kehtivuseAlgus", "sortAscending": false,
		},
		"precise": map[string]any{
			"kohtunik": nil, "ecliNr": "", "menetluseTyyp": nil, "annotatsiooniSisu": nil, "kohus": []int{}, "menetlusliigid": []int{},
			"seaduseSatted": map[string]any{"paragrahv": nil, "paragrahvPrimm": nil, "loige": nil, "loigePrimm": nil, "punkt": nil, "punktPrimm": nil, "seadustikKl": nil},
		},
	}
	out, _ := json.Marshal(b)
	return out
}

// Otsi otsib lahendeid.
func (c *Klient) Otsi(ctx context.Context, v Valikud) (*OtsinguVastus, error) {
	paring, sisus := v.Tekst, true
	if v.AsjaNr != "" {
		paring, sisus = baasNr(v.AsjaNr), false
	}
	if strings.TrimSpace(paring) == "" {
		return nil, viga.Uus(viga.Sisend, allikasRT, "", "otsingutekst või asja number on puudu", nil)
	}
	u := baas + "/kohtuteave/otsing/kohtulahendid"
	vs, err := c.k.Tee(ctx, klient.Paring{Allikas: allikasRT, Meetod: "POST", URL: u, Keha: otsinguKeha(paring, sisus, sisus), TTL: time.Hour, OodatudSisu: "json",
		Paised: map[string]string{"Content-Type": "application/json", "Accept": "application/json"}})
	if err != nil {
		return nil, err
	}
	var j tulemusJSON
	if err := json.Unmarshal(vs.Keha, &j); err != nil {
		return nil, viga.Uus(viga.StruktuurMuutunud, allikasRT, u, "otsingu vastus ei ole oodatud JSON", err)
	}
	if j.Kokku == nil {
		return nil, viga.Struktuur(allikasRT, u, "otsingu vastusest puudub väli 'kokku'")
	}
	if *j.Kokku > 0 && len(j.Tulemused) == 0 {
		return nil, viga.Struktuur(allikasRT, u, fmt.Sprintf("otsing teatas %d tulemust, kuid tulemuste loend on tühi", *j.Kokku))
	}
	out := &OtsinguVastus{Paring: paring, Kokku: *j.Kokku, Tulemused: []Lahend{}, AndmeURL: u}
	for _, t := range j.Tulemused {
		if t.AsjaNr == "" || t.ObjektID == 0 {
			return nil, viga.Struktuur(allikasRT, u, "tulemusest puudub kohtuasjaNumber või objektId")
		}
		l := Lahend{AsjaNr: t.AsjaNr, Kohus: t.Kohus, Kuupaev: kohalik(t.Aeg), Menetlus: t.Menetlus, Staatus: t.Staatus, URL: fmt.Sprintf("%s%d", detailidURL, t.ObjektID), objektID: t.ObjektID, failiID: t.FailiID}
		for _, k := range t.Kat {
			l.Kategooriad = append(l.Kategooriad, strings.Trim(k.Kat+" / "+k.Alam, " /"))
		}
		if !sobibAste(l.Kohus, v.Aste) || (v.Alates != "" && l.Kuupaev < v.Alates) || (v.Kuni != "" && l.Kuupaev > v.Kuni) {
			continue
		}
		out.Tulemused = append(out.Tulemused, l)
	}
	if v.Aste != "" || v.Alates != "" || v.Kuni != "" {
		out.Markus = "aste ja kuupäev on filtreeritud esimese tulemuslehe peal; 'kokku' on filtreerimata arv"
	}
	if v.Piir > 0 && len(out.Tulemused) > v.Piir {
		out.Tulemused = out.Tulemused[:v.Piir]
	}
	return out, nil
}

func sobibAste(kohus, aste string) bool {
	k := strings.ToLower(kohus)
	switch strings.ToLower(aste) {
	case "":
		return true
	case "riigikohus", "rk", "3":
		return strings.Contains(k, "riigikoh")
	case "ringkonnakohus", "rnk", "2":
		return strings.Contains(k, "ringkonnakoh")
	case "esimene", "1", "maakohus", "halduskohus":
		return strings.Contains(k, "maakoh") || strings.Contains(k, "halduskoh")
	}
	return true
}

func kohalik(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		if len(s) >= 10 {
			return s[:10]
		}
		return s
	}
	if tallinn != nil {
		t = t.In(tallinn)
	}
	return t.Format("2006-01-02")
}

var asjaNrRe = regexp.MustCompile(`^\d-\d{1,2}-\d{1,6}(?:-\d{1,4}){0,2}(?:/\d{1,4})?$`)

func baasNr(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "/"); i > 0 {
		return s[:i]
	}
	return s
}

// ---------- lahendi tekst ----------

// Paritolu kirjeldab teksti allikat.
type Paritolu struct {
	Allikas    string    `json:"allikas"`
	URL        string    `json:"url"`
	AndmeURL   string    `json:"andme_url"`
	Vorming    string    `json:"vorming"`
	Laaditud   time.Time `json:"laaditud"`
	SHA256     string    `json:"sha256"`
	Vahemalust bool      `json:"vahemalust"`
}

// TekstiVastus on lahendi tekst või selle valitud punktid.
type TekstiVastus struct {
	AsjaNr          string        `json:"asja_nr"`
	Kohus           string        `json:"kohus,omitempty"`
	Kuupaev         string        `json:"kuupaev,omitempty"`
	Punktid         []tekst.Punkt `json:"punktid,omitempty"`
	PuuduvadPunktid []int         `json:"puuduvad_punktid,omitempty"`
	PunkteKokku     int           `json:"punkte_kokku"`
	Tekst           string        `json:"tekst,omitempty"`
	Karbitud        bool          `json:"karbitud,omitempty"`
	Hoiatused       []string      `json:"hoiatused,omitempty"`
	Paritolu        Paritolu      `json:"paritolu"`
}

// TekstiValikud määravad, mida lahendist tagastada.
type TekstiValikud struct {
	AsjaNr   string
	Punktid  []int
	Ulevaade bool // iga punkti esimesed ~160 märki
	MaxMarke int  // täisteksti piir (vaikimisi 20000)
}

// Tekst laeb lahendi teksti.
func (c *Klient) Tekst(ctx context.Context, v TekstiValikud) (*TekstiVastus, error) {
	nr := strings.TrimSpace(v.AsjaNr)
	if !asjaNrRe.MatchString(nr) {
		return nil, viga.Uus(viga.Sisend, allikasRT, "", fmt.Sprintf("asja number %q ei vasta kujule 3-15-443/54", nr), nil)
	}
	lahend, otsinguViga := c.leiaDokument(ctx, nr)
	var raw string
	var par Paritolu
	var err error
	switch {
	case otsinguViga != nil && !viga.LiikOn(otsinguViga, viga.Blokeeritud) && !viga.LiikOn(otsinguViga, viga.Kattesaamatu):
		return nil, otsinguViga
	case lahend != nil && strings.Contains(strings.ToLower(lahend.Kohus), "riigikoh"):
		raw, par, err = c.riigikohtust(ctx, lahend.AsjaNr)
		if err != nil && lahend.failiID != 0 {
			// varutee: sama lahend Riigi Teataja PDFina
			raw, par, err = c.rtPDF(ctx, lahend)
		}
	case lahend != nil:
		raw, par, err = c.rtPDF(ctx, lahend)
	default:
		// Riigi Teataja otsing ei olnud kättesaadav: proovi Riigikohtu andmebaasi otse.
		raw, par, err = c.riigikohtust(ctx, nr)
		if err != nil {
			return nil, fmt.Errorf("%v; Riigikohtu andmebaasist ka ei leitud: %w", otsinguViga, err)
		}
	}
	if err != nil {
		return nil, err
	}
	if !strings.Contains(raw, baasNr(nr)) && !strings.Contains(raw, strings.ReplaceAll(baasNr(nr), "-", "‑")) {
		return nil, viga.Struktuur(par.Allikas, par.URL, "laaditud tekstis ei esine asja numbrit "+baasNr(nr)+" – vale dokument või muutunud vorming")
	}
	out := &TekstiVastus{AsjaNr: nr, Paritolu: par}
	if lahend != nil {
		out.AsjaNr, out.Kohus, out.Kuupaev = lahend.AsjaNr, lahend.Kohus, lahend.Kuupaev
	}
	punktid := tekst.JaotaPunktideks(raw)
	out.PunkteKokku = len(punktid)
	switch {
	case len(v.Punktid) > 0:
		if len(punktid) == 0 {
			return nil, viga.Struktuur(par.Allikas, par.URL, "lahendist ei õnnestunud eraldada nummerdatud punkte")
		}
		out.Punktid, out.PuuduvadPunktid = tekst.ValiPunktid(punktid, v.Punktid)
		if len(out.Punktid) == 0 {
			return nil, viga.Uus(viga.EiLeitud, par.Allikas, par.URL, fmt.Sprintf("soovitud punkte ei leitud; lahendis on punktid 1–%d", len(punktid)), nil)
		}
		if len(out.PuuduvadPunktid) > 0 {
			out.Hoiatused = append(out.Hoiatused, fmt.Sprintf("osa punkte puudub: %v", out.PuuduvadPunktid))
		}
	case v.Ulevaade:
		if len(punktid) == 0 {
			return nil, viga.Struktuur(par.Allikas, par.URL, "lahendist ei õnnestunud eraldada nummerdatud punkte")
		}
		for _, p := range punktid {
			r := []rune(p.Tekst)
			if len(r) > 160 {
				p.Tekst = string(r[:160]) + "…"
			}
			out.Punktid = append(out.Punktid, p)
		}
	default:
		max := v.MaxMarke
		if max <= 0 {
			max = 20000
		}
		r := []rune(raw)
		if len(r) > max {
			out.Tekst = string(r[:max])
			out.Karbitud = true
			out.Hoiatused = append(out.Hoiatused, fmt.Sprintf("tekst kärbiti %d märgini %d-st; küsi konkreetseid punkte või ülevaadet", max, len(r)))
		} else {
			out.Tekst = raw
		}
	}
	return out, nil
}

// leiaDokument otsib asja numbri järgi täpse dokumendi.
func (c *Klient) leiaDokument(ctx context.Context, nr string) (*Lahend, error) {
	res, err := c.Otsi(ctx, Valikud{AsjaNr: nr})
	if err != nil {
		return nil, err
	}
	var vasted []Lahend
	for _, l := range res.Tulemused {
		if l.AsjaNr == nr {
			return &l, nil
		}
		if baasNr(l.AsjaNr) == baasNr(nr) {
			vasted = append(vasted, l)
		}
	}
	if strings.Contains(nr, "/") || len(vasted) == 0 {
		return nil, viga.Uus(viga.EiLeitud, allikasRT, res.AndmeURL, fmt.Sprintf("lahendit %s ei leitud Riigi Teatajast", nr), nil)
	}
	if len(vasted) == 1 {
		return &vasted[0], nil
	}
	var nimed []string
	for _, l := range vasted {
		nimed = append(nimed, l.AsjaNr+" ("+l.Kohus+", "+l.Kuupaev+")")
	}
	return nil, viga.Uus(viga.Sisend, allikasRT, res.AndmeURL, "asjas on mitu lahendit, täpsusta numbrit: "+strings.Join(nimed, "; "), nil)
}

func (c *Klient) rtPDF(ctx context.Context, l *Lahend) (string, Paritolu, error) {
	if l.failiID == 0 {
		return "", Paritolu{}, viga.Uus(viga.EiLeitud, allikasRT, l.URL, "lahendil puudub avalikustatud fail", nil)
	}
	u := fmt.Sprintf("%s/kohtuteave/kohtulahendid/%d/file", baas, l.failiID)
	vs, err := c.k.Tee(ctx, klient.Paring{Allikas: allikasRT, URL: u, TTL: 30 * 24 * time.Hour, OodatudSisu: "pdf"})
	if err != nil {
		return "", Paritolu{}, err
	}
	t, err := tekst.PDFTekstiks(ctx, vs.Keha)
	if err != nil {
		if err == tekst.ErrPDFTeisendajaPuudub {
			return "", Paritolu{}, viga.Uus(viga.Soltuvus, allikasRT, u, err.Error(), nil)
		}
		return "", Paritolu{}, viga.Uus(viga.StruktuurMuutunud, allikasRT, u, "PDF teisendamine ebaõnnestus", err)
	}
	if len([]rune(t)) < 200 {
		return "", Paritolu{}, viga.Struktuur(allikasRT, u, "PDFist saadi alla 200 märgi teksti (skannitud dokument?)")
	}
	return t, Paritolu{Allikas: "Riigi Teataja", URL: l.URL, AndmeURL: u, Vorming: "pdf→pdftotext", Laaditud: vs.Laaditud, SHA256: vs.SHA256, Vahemalust: vs.Vahemalust}, nil
}

// riigikohtust laeb lahendi Riigikohtu lahendite andmebaasist (RIKOS).
func (c *Klient) riigikohtust(ctx context.Context, nr string) (string, Paritolu, error) {
	u := rikos + "?asjaNr=" + url.QueryEscape(nr)
	vs, err := c.k.Tee(ctx, klient.Paring{Allikas: allikasRK, URL: u, TTL: 30 * 24 * time.Hour, OodatudSisu: "html"})
	if err != nil {
		return "", Paritolu{}, err
	}
	if strings.Contains(vs.URL, "EriVaade") {
		return "", Paritolu{}, viga.Uus(viga.Sisend, allikasRK, vs.URL, "asjas on Riigikohtus mitu dokumenti; täpsusta numbrit (nt /54)", nil)
	}
	t, err := tekst.HTMLTekstiks(vs.Keha)
	if err != nil {
		return "", Paritolu{}, viga.Uus(viga.StruktuurMuutunud, allikasRK, vs.URL, "HTML ei ole loetav", err)
	}
	if strings.Contains(t, "Tulemusi leiti kokku: 0") || strings.Contains(t, "Antud otsing ei andud tulemusi") {
		return "", Paritolu{}, viga.Uus(viga.EiLeitud, allikasRK, vs.URL, "Riigikohtu andmebaasis lahendit ei leitud", nil)
	}
	if !strings.Contains(t, "RIIGIKOHUS") {
		return "", Paritolu{}, viga.Struktuur(allikasRK, vs.URL, "lehel puudub pealkiri RIIGIKOHUS – vale leht või muutunud vorming")
	}
	if i := strings.Index(t, baasNr(nr)); i > 0 {
		t = t[i:]
	}
	return t, Paritolu{Allikas: "Riigikohus (RIKOS)", URL: "https://www.riigikohus.ee/et/lahendid?asjaNr=" + url.QueryEscape(nr), AndmeURL: vs.URL, Vorming: "html", Laaditud: vs.Laaditud, SHA256: vs.SHA256, Vahemalust: vs.Vahemalust}, nil
}
