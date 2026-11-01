// Package eurlex laeb ELi õigusakte ja Euroopa Kohtu lahendeid ELi
// Väljaannete Talituse Cellar REST liidesest (publications.europa.eu).
// Cellar on EUR-Lexi andmete ametlik masinloetav allikas; eur-lex.europa.eu
// HTML-lehed on AWS WAF kontrolli taga ja neid tööriist ei kasuta.
package eurlex

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Ker102/oigusallikad/internal/klient"
	"github.com/Ker102/oigusallikad/internal/tekst"
	"github.com/Ker102/oigusallikad/internal/viga"
)

const (
	allikas = "eurlex-cellar"
)

// Aadressid on muutujad, et teste saaks käivitada kohaliku serveri vastu.
var (
	cellar = "https://publications.europa.eu/resource/celex/"
	eurlex = "https://eur-lex.europa.eu/legal-content/%s/TXT/?uri=CELEX:%s"
)

// Klient on Cellari klient.
type Klient struct{ k *klient.Klient }

// Uus loob kliendi.
func Uus(k *klient.Klient) *Klient { return &Klient{k: k} }

var keeled = map[string]string{
	"ET": "est", "EN": "eng", "DE": "deu", "FR": "fra", "FI": "fin", "LV": "lav", "LT": "lit", "SV": "swe", "PL": "pol", "IT": "ita", "ES": "spa", "NL": "nld", "DA": "dan", "CS": "ces", "SK": "slk", "SL": "slv", "HU": "hun", "PT": "por", "RO": "ron", "BG": "bul", "EL": "ell", "HR": "hrv", "MT": "mlt", "GA": "gle",
}

var celexRe = regexp.MustCompile(`^[0-9CE][0-9]{4}[A-Z]{1,2}[0-9A-Z()_.\-]{1,30}$`)

// kohtuasiRe: C-626/22, T-123/19, C‑487/17 (sidekriipsu variandid).
var kohtuasiRe = regexp.MustCompile(`^(?i)([CT])\s*[-‑–]\s*(\d{1,4})\s*/\s*(\d{2})$`)

// CELEXKohtuasjast teisendab kohtuasja numbri CELEX numbriks:
// liik "otsus" (CJ/TJ), "ettepanek" (kohtujuristi ettepanek, CC) või "maarus" (CO/TO).
func CELEXKohtuasjast(asi, liik string) (string, error) {
	m := kohtuasiRe.FindStringSubmatch(strings.TrimSpace(asi))
	if m == nil {
		return "", fmt.Errorf("kohtuasja number peab olema kujul C-626/22")
	}
	yy, _ := strconv.Atoi(m[3])
	aasta := 2000 + yy
	if yy >= 50 {
		aasta = 1900 + yy
	}
	kohus := strings.ToUpper(m[1])
	var tyyp string
	switch strings.ToLower(liik) {
	case "", "otsus", "kohtuotsus":
		tyyp = kohus + "J"
	case "ettepanek", "kohtujurist":
		if kohus != "C" {
			return "", fmt.Errorf("kohtujuristi ettepanekud on ainult Euroopa Kohtu (C-) asjades")
		}
		tyyp = "CC"
	case "maarus", "määrus":
		tyyp = kohus + "O"
	default:
		return "", fmt.Errorf("tundmatu liik %q (otsus, ettepanek, maarus)", liik)
	}
	nr, _ := strconv.Atoi(m[2])
	return fmt.Sprintf("6%d%s%04d", aasta, tyyp, nr), nil
}

// Valikud määravad, mida dokumendist tagastada.
type Valikud struct {
	Viide    string // CELEX või kohtuasja number (C-626/22)
	Liik     string // kohtuasja puhul: otsus, ettepanek, maarus
	Keel     string // vaikimisi ET
	Artiklid []string
	Punktid  []int
	// Allmarkused: allmärkuste numbrid (kohtujuristi ettepanekutes on oluline
	// sisu sageli allmärkustes).
	Allmarkused []int
	MaxMarke    int
}

// Osa on artikkel või kohtuotsuse punkt.
type Osa struct {
	Viide string `json:"viide"`
	Tekst string `json:"tekst"`
}

// Paritolu kirjeldab allikat.
type Paritolu struct {
	Allikas    string    `json:"allikas"`
	URL        string    `json:"url"`
	AndmeURL   string    `json:"andme_url"`
	Laaditud   time.Time `json:"laaditud"`
	SHA256     string    `json:"sha256"`
	Vahemalust bool      `json:"vahemalust"`
}

// Vastus on dokument või selle osad.
type Vastus struct {
	CELEX     string   `json:"celex"`
	Keel      string   `json:"keel"`
	Pealkiri  string   `json:"pealkiri"`
	Osad      []Osa    `json:"osad,omitempty"`
	Puuduvad  []string `json:"puuduvad,omitempty"`
	Tekst     string   `json:"tekst,omitempty"`
	Karbitud  bool     `json:"karbitud,omitempty"`
	Hoiatused []string `json:"hoiatused,omitempty"`
	Paritolu  Paritolu `json:"paritolu"`
}

// Dokument laeb dokumendi Cellarist.
func (c *Klient) Dokument(ctx context.Context, v Valikud) (*Vastus, error) {
	celex := strings.ToUpper(strings.TrimSpace(v.Viide))
	celex = strings.TrimPrefix(celex, "CELEX:")
	if kohtuasiRe.MatchString(v.Viide) {
		var err error
		if celex, err = CELEXKohtuasjast(v.Viide, v.Liik); err != nil {
			return nil, viga.Uus(viga.Sisend, allikas, "", err.Error(), nil)
		}
	}
	if !celexRe.MatchString(celex) {
		return nil, viga.Uus(viga.Sisend, allikas, "", fmt.Sprintf("%q ei ole CELEX number (nt 32008L0098, 62022CJ0626) ega kohtuasja number (C-626/22)", v.Viide), nil)
	}
	keel := strings.ToUpper(v.Keel)
	if keel == "" {
		keel = "ET"
	}
	k3, ok := keeled[keel]
	if !ok {
		return nil, viga.Uus(viga.Sisend, allikas, "", "tundmatu keelekood "+keel, nil)
	}
	u := cellar + celex
	// Vanemad dokumendid on Cellaris XHTML-ina, uuemad (nt 2025+ kohtujuristi
	// ettepanekud) HTML-ina. Proovime mõlemat.
	var vs *klient.Vastus
	var err error
	for _, accept := range []string{"application/xhtml+xml", "text/html"} {
		vs, err = c.k.Tee(ctx, klient.Paring{Allikas: allikas, URL: u, TTL: 7 * 24 * time.Hour, OodatudSisu: "html",
			Paised: map[string]string{"Accept": accept, "Accept-Language": k3}})
		if err == nil || !viga.LiikOn(err, viga.EiLeitud) {
			break
		}
	}
	if err != nil {
		if viga.LiikOn(err, viga.EiLeitud) {
			return nil, viga.Uus(viga.EiLeitud, allikas, u, fmt.Sprintf("dokumenti %s ei ole Cellaris keeles %s HTML-vormingus: kontrolli CELEX numbrit või proovi teist keelt (värsked lahendid ilmuvad sageli esmalt FR/EN keeles); vanematel dokumentidel võib olla ainult PDF", celex, keel), nil)
		}
		return nil, err
	}
	html := vs.Keha
	t, err := tekst.HTMLTekstiks(html)
	if err != nil {
		return nil, viga.Uus(viga.StruktuurMuutunud, allikas, vs.URL, "XHTML ei ole loetav", err)
	}
	if len([]rune(t)) < 300 {
		return nil, viga.Struktuur(allikas, vs.URL, "dokumendi tekst on alla 300 märgi")
	}
	out := &Vastus{CELEX: celex, Keel: keel, Pealkiri: pealkiri(t), Paritolu: Paritolu{Allikas: "EUR-Lex (Cellar)", URL: fmt.Sprintf(eurlex, keel, celex), AndmeURL: vs.URL, Laaditud: vs.Laaditud, SHA256: vs.SHA256, Vahemalust: vs.Vahemalust}}

	if strings.Contains(strings.ToLower(t[:min(len(t), 3000)]), "esialgne tõlge") || strings.Contains(strings.ToLower(t[:min(len(t), 3000)]), "provisional text") {
		out.Hoiatused = append(out.Hoiatused, "ESIALGNE TÕLGE: sõnastus ja punktide numeratsioon võivad lõplikus versioonis muutuda; tsiteerimisel kontrolli")
	}
	if len(v.Allmarkused) > 0 {
		am := allmarkused(html)
		if len(am) == 0 {
			return nil, viga.Struktuur(allikas, vs.URL, "dokumendis puuduvad allmärkuste ankrud (NAME=\"FootnoteN\")")
		}
		for _, n := range v.Allmarkused {
			if x, ok := am[n]; ok {
				out.Osad = append(out.Osad, Osa{Viide: "allmärkus " + strconv.Itoa(n), Tekst: x})
			} else {
				out.Puuduvad = append(out.Puuduvad, "allmärkus "+strconv.Itoa(n))
			}
		}
	}

	switch {
	case len(v.Allmarkused) > 0 && len(v.Punktid) == 0 && len(v.Artiklid) == 0:
		if len(out.Osad) == 0 {
			return nil, viga.Uus(viga.EiLeitud, allikas, vs.URL, fmt.Sprintf("soovitud allmärkusi ei leitud; dokumendis on %d allmärkust", len(allmarkused(html))), nil)
		}
	case len(v.Punktid) > 0:
		punktid := kohtuotsusePunktid(html)
		if len(punktid) == 0 {
			return nil, viga.Struktuur(allikas, vs.URL, "dokumendis puuduvad punktide ankrud (id/name=\"pointN\"); kas see on kohtulahend?")
		}
		for _, n := range v.Punktid {
			if p, ok := punktid[n]; ok {
				out.Osad = append(out.Osad, Osa{Viide: "p " + strconv.Itoa(n), Tekst: p})
			} else {
				out.Puuduvad = append(out.Puuduvad, "p "+strconv.Itoa(n))
			}
		}
		if len(out.Osad) == 0 {
			return nil, viga.Uus(viga.EiLeitud, allikas, vs.URL, fmt.Sprintf("soovitud punkte ei leitud; dokumendis on %d punkti", len(punktid)), nil)
		}
	case len(v.Artiklid) > 0:
		artiklid := artiklidTekstist(t)
		if len(artiklid) == 0 {
			return nil, viga.Struktuur(allikas, vs.URL, "dokumendis ei leitud ühtegi rida 'Artikkel N' / 'Article N'")
		}
		for _, a := range v.Artiklid {
			key := strings.ToLower(strings.TrimSpace(a))
			if x, ok := artiklid[key]; ok {
				out.Osad = append(out.Osad, Osa{Viide: "art " + a, Tekst: x})
			} else {
				out.Puuduvad = append(out.Puuduvad, "art "+a)
			}
		}
		if len(out.Osad) == 0 {
			return nil, viga.Uus(viga.EiLeitud, allikas, vs.URL, fmt.Sprintf("soovitud artikleid ei leitud; dokumendis on %d artiklit", len(artiklid)), nil)
		}
	default:
		max := v.MaxMarke
		if max <= 0 {
			max = 20000
		}
		r := []rune(t)
		if len(r) > max {
			out.Tekst = string(r[:max])
			out.Karbitud = true
			out.Hoiatused = append(out.Hoiatused, fmt.Sprintf("tekst kärbiti %d märgini %d-st; küsi konkreetseid artikleid või punkte", max, len(r)))
		} else {
			out.Tekst = t
		}
	}
	if len(out.Puuduvad) > 0 {
		out.Hoiatused = append(out.Hoiatused, "osa soovitud osi puudub: "+strings.Join(out.Puuduvad, ", "))
	}
	return out, nil
}

func pealkiri(t string) string {
	var osad []string
	for _, l := range strings.Split(t, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || celexRe.MatchString(l) || strings.HasPrefix(l, "▼") {
			continue
		}
		osad = append(osad, l)
		if len(strings.Join(osad, " ")) > 120 || len(osad) == 3 {
			break
		}
	}
	s := strings.Join(osad, " ")
	if r := []rune(s); len(r) > 300 {
		s = string(r[:300]) + "…"
	}
	return s
}

// Punktide ankrud: vanem/keskmine vorming <p id="pointN">, uuem <A NAME="pointN">.
var punktAnkur = regexp.MustCompile(`<[^>]+\b(?i:id|name)="point(\d{1,4})"[^>]*>`)

// Allmärkuste sisu: <A HREF="#FootrefN" NAME="FootnoteN">N</A> tekst.
var allmarkuseAnkur = regexp.MustCompile(`<[^>]+\b(?i:id|name)="Footnote(\d{1,4})"[^>]*>`)

func allmarkused(html []byte) map[int]string {
	locs := allmarkuseAnkur.FindAllSubmatchIndex(html, -1)
	out := map[int]string{}
	for i, loc := range locs {
		n, _ := strconv.Atoi(string(html[loc[2]:loc[3]]))
		end := len(html)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		seg := html[loc[0]:end]
		// allmärkus lõpeb lõigu lõpuga
		if j := bytes.Index(bytes.ToLower(seg), []byte("</p>")); j > 0 {
			seg = seg[:j]
		}
		t, err := tekst.HTMLTekstiks(seg)
		if err != nil {
			continue
		}
		t = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(t), strconv.Itoa(n)))
		if _, dup := out[n]; !dup && t != "" {
			out[n] = strings.Join(strings.Fields(t), " ")
		}
	}
	return out
}

// kohtuotsusePunktid eraldab Euroopa Kohtu lahendi punktid ankrute
// id="pointN" järgi (nii uues kui vanas Cellari XHTML-vormingus).
func kohtuotsusePunktid(html []byte) map[int]string {
	locs := punktAnkur.FindAllSubmatchIndex(html, -1)
	out := map[int]string{}
	for i, loc := range locs {
		n, _ := strconv.Atoi(string(html[loc[2]:loc[3]]))
		end := len(html)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		seg := html[loc[0]:end]
		t, err := tekst.HTMLTekstiks(seg)
		if err != nil {
			continue
		}
		t = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(t), strconv.Itoa(n)))
		t = strings.TrimSpace(strings.TrimPrefix(t, "."))
		// viimase punkti järel võib tulla resolutsioon, allkirjad ja allmärkused
		if i+1 == len(locs) {
			if j := bytes.Index(bytes.ToLower(seg), []byte("<hr")); j > 0 {
				if t2, err := tekst.HTMLTekstiks(seg[:j]); err == nil {
					t = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(t2), strconv.Itoa(n)), "."))
				}
			}
			for _, stop := range []string{"\nKohtukulud", "\nCosts", "\nAllkirjad", "\nSignatures"} {
				if j := strings.Index(t, stop); j > 0 {
					t = t[:j]
				}
			}
		}
		if _, dup := out[n]; !dup {
			out[n] = strings.Join(strings.Fields(t), " ")
		}
	}
	return out
}

var artikliRida = regexp.MustCompile(`^(?:Artikkel|Article|Artikel|Artículo|Articolo)\s+(\d{1,3}[a-z]?)$`)

// artiklidTekstist jaotab õigusakti artikliteks ridade "Artikkel N" järgi.
// Kui sama artikkel esineb mitu korda (nt sisukorras), võetakse pikim.
func artiklidTekstist(t string) map[string]string {
	lines := strings.Split(t, "\n")
	type alg struct {
		nr  string
		idx int
	}
	var algused []alg
	for i, l := range lines {
		if m := artikliRida.FindStringSubmatch(strings.TrimSpace(l)); m != nil {
			algused = append(algused, alg{strings.ToLower(m[1]), i})
		}
	}
	out := map[string]string{}
	for i, a := range algused {
		end := len(lines)
		if i+1 < len(algused) {
			end = algused[i+1].idx
		}
		seg := lines[a.idx:end]
		// lisa algus lõpetab viimase artikli
		for j, l := range seg {
			ul := strings.ToUpper(strings.TrimSpace(l))
			if j > 0 && (strings.HasPrefix(ul, "LISA ") || ul == "LISA" || strings.HasPrefix(ul, "ANNEX") || strings.HasPrefix(ul, "I LISA") || strings.HasPrefix(ul, "II LISA")) {
				seg = seg[:j]
				break
			}
		}
		txt := strings.TrimSpace(strings.Join(seg, "\n"))
		if len(txt) > len(out[a.nr]) {
			out[a.nr] = txt
		}
	}
	return out
}

// ArtikliteLoend tagastab artiklite numbrid (testimiseks ja ülevaateks).
func ArtikliteLoend(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		a, _ := strconv.Atoi(strings.TrimRight(out[i], "abcdefghijklmnopqrstuvwxyz"))
		b, _ := strconv.Atoi(strings.TrimRight(out[j], "abcdefghijklmnopqrstuvwxyz"))
		if a != b {
			return a < b
		}
		return out[i] < out[j]
	})
	return out
}
