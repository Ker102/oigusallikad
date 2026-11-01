// Package rt loeb Riigi Teataja õigusakte avaliku API kaudu
// (www.riigiteataja.ee/public-api). Tekst võetakse ametlikust XMList
// (tyviseadus/maarus skeem), mitte HTML-lehelt.
package rt

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // Europe/Tallinn ka ilma süsteemse tsooniandmebaasita
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/Ker102/oigusallikad/internal/klient"
	"github.com/Ker102/oigusallikad/internal/tekst"
	"github.com/Ker102/oigusallikad/internal/viga"
)

const (
	allikas = "riigiteataja"
)

// Aadressid on muutujad, et teste saaks käivitada kohaliku serveri vastu.
var (
	baas   = "https://www.riigiteataja.ee/public-api/api/v1"
	avalik = "https://www.riigiteataja.ee/akt/"
)

var tallinn = func() *time.Location {
	l, err := time.LoadLocation("Europe/Tallinn")
	if err != nil {
		return time.FixedZone("EET", 2*3600)
	}
	return l
}()

// RT on Riigi Teataja klient.
type RT struct{ k *klient.Klient }

// Uus loob kliendi.
func Uus(k *klient.Klient) *RT { return &RT{k: k} }

// Paritolu kirjeldab, kust tekst pärineb.
type Paritolu struct {
	Allikas    string    `json:"allikas"`
	URL        string    `json:"url"`
	AndmeURL   string    `json:"andme_url"`
	Laaditud   time.Time `json:"laaditud"`
	SHA256     string    `json:"sha256"`
	Vahemalust bool      `json:"vahemalust"`
}

// Akt on akti redaktsiooni metaandmed.
type Akt struct {
	ID           int64  `json:"id"`
	GrupiID      int64  `json:"grupi_id"`
	Pealkiri     string `json:"pealkiri"`
	Lyhend       string `json:"lyhend,omitempty"`
	Liik         string `json:"liik"`
	Tekstiliik   string `json:"tekstiliik"`
	KehtivAlates string `json:"kehtiv_alates"`
	KehtivKuni   string `json:"kehtiv_kuni,omitempty"`
	URL          string `json:"url"`
	lisaAadress  string
}

// Lyhend on seaduse lühend.
type Lyhend struct {
	ID       int64  `json:"id"`
	Lyhend   string `json:"lyhend"`
	Pealkiri string `json:"pealkiri"`
}

func (r *RT) getJSON(ctx context.Context, u string, ttl time.Duration, v any) (*klient.Vastus, error) {
	vs, err := r.k.Tee(ctx, klient.Paring{Allikas: allikas, URL: u, TTL: ttl, OodatudSisu: "json", Paised: map[string]string{"Accept": "application/json"}})
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(vs.Keha, v); err != nil {
		return nil, viga.Uus(viga.StruktuurMuutunud, allikas, u, "JSON ei vasta oodatud kujule", err)
	}
	return vs, nil
}

// Lyhendid tagastab kõigi seaduste lühendid.
func (r *RT) Lyhendid(ctx context.Context) ([]Lyhend, error) {
	u := baas + "/akt/lyhendid"
	var out []Lyhend
	if _, err := r.getJSON(ctx, u, 24*time.Hour, &out); err != nil {
		return nil, err
	}
	if len(out) < 100 {
		return nil, viga.Struktuur(allikas, u, fmt.Sprintf("lühendite loend on kahtlaselt lühike (%d)", len(out)))
	}
	for _, l := range out[:10] {
		if l.ID == 0 || l.Lyhend == "" || l.Pealkiri == "" {
			return nil, viga.Struktuur(allikas, u, "lühendi kirjest puuduvad väljad id/lyhend/pealkiri")
		}
	}
	return out, nil
}

var (
	idRe     = regexp.MustCompile(`^\d{6,13}$`)
	aktUrlRe = regexp.MustCompile(`riigiteataja\.ee/(?:et/|en/)?akt/([^/?#]+)`)
)

func normaliseeri(s string) string {
	return strings.ToLower(norm.NFC.String(strings.TrimSpace(s)))
}

// Lahenda leiab akti ID lühendi, ID, Riigi Teataja lingi või pealkirja järgi.
func (r *RT) Lahenda(ctx context.Context, paring string) (int64, error) {
	p := strings.TrimSpace(paring)
	if p == "" {
		return 0, viga.Uus(viga.Sisend, allikas, "", "akt on määramata", nil)
	}
	if m := aktUrlRe.FindStringSubmatch(p); m != nil {
		p, _ = url.PathUnescape(m[1])
	}
	if idRe.MatchString(p) {
		id, _ := strconv.ParseInt(p, 10, 64)
		return id, nil
	}
	lyh, err := r.Lyhendid(ctx)
	if err != nil {
		return 0, err
	}
	np := normaliseeri(p)
	for _, l := range lyh {
		if normaliseeri(l.Lyhend) == np {
			return l.ID, nil
		}
	}
	var vasted []Lyhend
	for _, l := range lyh {
		if normaliseeri(l.Pealkiri) == np {
			return l.ID, nil
		}
		if strings.Contains(normaliseeri(l.Pealkiri), np) {
			vasted = append(vasted, l)
		}
	}
	switch len(vasted) {
	case 1:
		return vasted[0].ID, nil
	case 0:
		return 0, viga.Uus(viga.EiLeitud, allikas, "", fmt.Sprintf("akti %q ei leitud seaduste lühendite ja pealkirjade hulgast. Määrustel lühend puudub: anna Riigi Teataja link või akti ID (pealkirjaotsing on Riigi Teatajas Cloudflare'i kontrolli taga)", paring), nil)
	default:
		var nimed []string
		for i, v := range vasted {
			if i == 10 {
				nimed = append(nimed, "…")
				break
			}
			nimed = append(nimed, v.Lyhend+" ("+v.Pealkiri+")")
		}
		return 0, viga.Uus(viga.Sisend, allikas, "", fmt.Sprintf("päring %q sobib mitmele aktile: %s", paring, strings.Join(nimed, "; ")), nil)
	}
}

type metaJSON struct {
	KehtivID   int64  `json:"kehtivId"`
	GrupiID    int64  `json:"grupiId"`
	Tekstiliik string `json:"tekstiliik"`
	Liik       string `json:"dokumentliik"`
	Param      struct {
		Pealkiri       string  `json:"pealkiri"`
		Lyhend         *string `json:"lyhend"`
		LisaAadress    string  `json:"lisaAadress"`
		KehtivuseAlgus *string `json:"kehtivuseAlgus"`
		KehtivuseLopp  *string `json:"kehtivuseLopp"`
	} `json:"aktiParameetrid"`
}

// Meta tagastab akti redaktsiooni metaandmed.
func (r *RT) Meta(ctx context.Context, id int64) (*Akt, error) {
	u := fmt.Sprintf("%s/akt/%d?leiaKehtiv=false&otsiTolkeSeos=true", baas, id)
	var m metaJSON
	if _, err := r.getJSON(ctx, u, 6*time.Hour, &m); err != nil {
		return nil, err
	}
	if m.GrupiID == 0 || m.Param.Pealkiri == "" {
		return nil, viga.Struktuur(allikas, u, "metaandmetest puudub grupiId või pealkiri")
	}
	a := &Akt{ID: id, GrupiID: m.GrupiID, Pealkiri: m.Param.Pealkiri, Liik: m.Liik, Tekstiliik: m.Tekstiliik, URL: fmt.Sprintf("%s%d", avalik, id), lisaAadress: m.Param.LisaAadress}
	if m.Param.Lyhend != nil {
		a.Lyhend = *m.Param.Lyhend
	}
	if m.Param.KehtivuseAlgus != nil {
		a.KehtivAlates = kohalikKuupaev(*m.Param.KehtivuseAlgus)
	}
	if m.Param.KehtivuseLopp != nil {
		a.KehtivKuni = kohalikKuupaev(*m.Param.KehtivuseLopp)
	}
	return a, nil
}

// kohalikKuupaev teisendab UTC aja ("2026-09-30T21:00:00Z") Tallinna kuupäevaks.
func kohalikKuupaev(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	return t.In(tallinn).Format("2006-01-02")
}

// Redaktsioon on akti terviktekst teatud ajavahemikus.
type Redaktsioon struct {
	ID           int64  `json:"id"`
	KehtivAlates string `json:"kehtiv_alates"`
	KehtivKuni   string `json:"kehtiv_kuni,omitempty"`
}

// Redaktsioonid tagastab akti kõik redaktsioonid uuemad eespool.
func (r *RT) Redaktsioonid(ctx context.Context, grupiID int64) ([]Redaktsioon, error) {
	u := fmt.Sprintf("%s/akt/%d/redaktsioonid", baas, grupiID)
	var m struct {
		Redaktsioonid []struct {
			ID             int64   `json:"id"`
			KehtivuseAlgus *string `json:"kehtivuseAlgus"`
		} `json:"redaktsioonid"`
	}
	if _, err := r.getJSON(ctx, u, 6*time.Hour, &m); err != nil {
		return nil, err
	}
	if len(m.Redaktsioonid) == 0 {
		return nil, viga.Struktuur(allikas, u, "redaktsioonide loend on tühi")
	}
	var out []Redaktsioon
	for _, x := range m.Redaktsioonid {
		if x.ID == 0 || x.KehtivuseAlgus == nil {
			continue
		}
		out = append(out, Redaktsioon{ID: x.ID, KehtivAlates: kohalikKuupaev(*x.KehtivuseAlgus)})
	}
	if len(out) == 0 {
		return nil, viga.Struktuur(allikas, u, "redaktsioonidel puudub id või kehtivuseAlgus")
	}
	sort.Slice(out, func(i, j int) bool { return out[i].KehtivAlates > out[j].KehtivAlates })
	for i := 1; i < len(out); i++ {
		if t, err := time.Parse("2006-01-02", out[i-1].KehtivAlates); err == nil {
			out[i].KehtivKuni = t.AddDate(0, 0, -1).Format("2006-01-02")
		}
	}
	return out, nil
}

// ValiRedaktsioon leiab akti redaktsiooni, mis kehtis antud kuupäeval
// (YYYY-MM-DD). Tühi kuupäev = tänane.
func (r *RT) ValiRedaktsioon(ctx context.Context, id int64, kuupaev string) (*Akt, error) {
	meta, err := r.Meta(ctx, id)
	if err != nil {
		return nil, err
	}
	if kuupaev == "" {
		kuupaev = time.Now().In(tallinn).Format("2006-01-02")
	}
	if _, err := time.Parse("2006-01-02", kuupaev); err != nil {
		return nil, viga.Uus(viga.Sisend, allikas, "", "kuupäev peab olema kujul AAAA-KK-PP", err)
	}
	reds, err := r.Redaktsioonid(ctx, meta.GrupiID)
	if err != nil {
		return nil, err
	}
	for _, rd := range reds {
		if rd.KehtivAlates <= kuupaev {
			if rd.ID == id {
				return meta, nil
			}
			return r.Meta(ctx, rd.ID)
		}
	}
	return nil, viga.Uus(viga.EiLeitud, allikas, meta.URL, fmt.Sprintf("aktil puudub %s kehtinud redaktsioon (vanim algab %s)", kuupaev, reds[len(reds)-1].KehtivAlates), nil)
}

// tekstiXML laeb redaktsiooni XMLi ja kontrollib juurelementi.
func (r *RT) tekstiXML(ctx context.Context, id int64) (*solm, *klient.Vastus, error) {
	u := fmt.Sprintf("%s/akt/%d/blob-xml", baas, id)
	vs, err := r.k.Tee(ctx, klient.Paring{Allikas: allikas, URL: u, TTL: 24 * time.Hour, OodatudSisu: "xml"})
	if err != nil {
		return nil, nil, err
	}
	doc, err := parsiXML(vs.Keha)
	if err != nil {
		return nil, nil, viga.Uus(viga.StruktuurMuutunud, allikas, u, "XML ei ole loetav", err)
	}
	if doc.laps("oigusakt") == nil {
		return nil, nil, viga.Struktuur(allikas, u, "XML juurelement ei ole <oigusakt>")
	}
	if len(doc.otsi("paragrahv")) == 0 {
		return nil, nil, viga.Struktuur(allikas, u, "XML-is ei ole ühtegi <paragrahv> elementi")
	}
	return doc, vs, nil
}

func paritolu(a *Akt, vs *klient.Vastus) Paritolu {
	return Paritolu{Allikas: "Riigi Teataja", URL: a.URL, AndmeURL: vs.URL, Laaditud: vs.Laaditud, SHA256: vs.SHA256, Vahemalust: vs.Vahemalust}
}

// ---------- sätted ----------

// SatteVastus on paragrahvi (või selle osa) tekst.
type SatteVastus struct {
	Akt      Akt      `json:"akt"`
	Viide    string   `json:"viide"`
	Tekst    string   `json:"tekst"`
	Muudetud []string `json:"muudetud,omitempty"`
	Paritolu Paritolu `json:"paritolu"`
}

var paraRe = regexp.MustCompile(`^(\d{1,4})(?:\s*(?:\^|-|prim\s*)?\s*(\d{1,2}))?$`)

// ParsiNumber loeb "88", "88^1", "88¹" või "88 prim 1" kujul numbri ja ülaindeksi.
func ParsiNumber(s string) (nr, yla string, ok bool) {
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "§"))
	var b strings.Builder
	for _, r := range s {
		found := false
		for d, u := range ylaindeks {
			if r == u {
				b.WriteRune('^')
				b.WriteRune(d)
				found = true
				break
			}
		}
		if !found {
			b.WriteRune(r)
		}
	}
	s = strings.ReplaceAll(b.String(), "^^", "^")
	m := paraRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

func vastab(s *solm, silt, nr, yla string) bool {
	n := s.laps(silt)
	if n == nil {
		return false
	}
	return strings.TrimSpace(n.tekstSisu()) == nr && strings.TrimSpace(n.attr["ylaIndeks"]) == yla
}

// Satted tagastab paragrahvi teksti; loige ja punkt kitsendavad valikut.
func (r *RT) Satted(ctx context.Context, akt, paragrahv, loige, punkt, kuupaev string) (*SatteVastus, error) {
	id, err := r.Lahenda(ctx, akt)
	if err != nil {
		return nil, err
	}
	a, err := r.ValiRedaktsioon(ctx, id, kuupaev)
	if err != nil {
		return nil, err
	}
	doc, vs, err := r.tekstiXML(ctx, a.ID)
	if err != nil {
		return nil, err
	}
	pnr, pyla, ok := ParsiNumber(paragrahv)
	if !ok {
		return nil, viga.Uus(viga.Sisend, allikas, "", "paragrahvi number peab olema nt 98, 88^1 või 88¹", nil)
	}
	var para *solm
	for _, p := range doc.otsi("paragrahv") {
		if vastab(p, "paragrahvNr", pnr, pyla) {
			para = p
			break
		}
	}
	viide := fmt.Sprintf("%s § %s", nimetus(a), pnr+ylaindeksiks(pyla))
	if para == nil {
		return nil, viga.Uus(viga.EiLeitud, allikas, a.URL, fmt.Sprintf("%s ei leitud (redaktsioon kehtiv alates %s)", viide, a.KehtivAlates), nil)
	}
	var sb strings.Builder
	muudetud := para.muutmismargid()
	pealkiri := textOf(para, "paragrahvPealkiri")
	loiked := para.koik("loige")
	if loige != "" {
		lnr, lyla, ok := ParsiNumber(loige)
		if !ok {
			return nil, viga.Uus(viga.Sisend, allikas, "", "lõike number peab olema nt 3 või 1^1", nil)
		}
		var valitud []*solm
		for _, l := range loiked {
			if vastab(l, "loigeNr", lnr, lyla) {
				valitud = append(valitud, l)
			}
		}
		if len(valitud) == 0 {
			return nil, viga.Uus(viga.EiLeitud, allikas, a.URL, fmt.Sprintf("%s lg %s ei leitud; paragrahvis on %d lõiget", viide, lnr+ylaindeksiks(lyla), len(loiked)), nil)
		}
		loiked = valitud
		viide += " lg " + lnr + ylaindeksiks(lyla)
	} else {
		sb.WriteString(para.kuvatavNr())
		if pealkiri != "" {
			sb.WriteString(" " + pealkiri)
		}
		sb.WriteString("\n")
	}
	for _, l := range loiked {
		muudetud = append(muudetud, l.muutmismargid()...)
		if punkt != "" {
			pn, py, ok := ParsiNumber(punkt)
			if !ok {
				return nil, viga.Uus(viga.Sisend, allikas, "", "punkti number peab olema nt 5 või 2^1", nil)
			}
			leitud := false
			for _, ap := range l.koik("alampunkt") {
				if vastab(ap, "alampunktNr", pn, py) {
					sb.WriteString(ap.kuvatavNr() + " " + textOf(ap, "sisuTekst") + "\n")
					muudetud = append(muudetud, ap.muutmismargid()...)
					leitud = true
				}
			}
			if !leitud {
				return nil, viga.Uus(viga.EiLeitud, allikas, a.URL, fmt.Sprintf("%s p %s ei leitud", viide, pn+ylaindeksiks(py)), nil)
			}
			viide += " p " + pn + ylaindeksiks(py)
			continue
		}
		kn := l.kuvatavNr()
		if kn != "" {
			sb.WriteString(kn + " ")
		}
		sb.WriteString(textOf(l, "sisuTekst") + "\n")
		for _, ap := range l.koik("alampunkt") {
			sb.WriteString(ap.kuvatavNr() + " " + textOf(ap, "sisuTekst") + "\n")
			muudetud = append(muudetud, ap.muutmismargid()...)
		}
	}
	tekst := strings.TrimSpace(sb.String())
	if tekst == "" {
		return nil, viga.Struktuur(allikas, a.URL, viide+" leiti, kuid tekst on tühi")
	}
	return &SatteVastus{Akt: *a, Viide: viide, Tekst: tekst, Muudetud: uniq(muudetud), Paritolu: paritolu(a, vs)}, nil
}

func nimetus(a *Akt) string {
	if a.Lyhend != "" {
		return a.Lyhend
	}
	// lühendita akt (määrus): lühendatud pealkiri, täispealkiri on väljal akt.pealkiri
	if r := []rune(a.Pealkiri); len(r) > 50 {
		return string(r[:50]) + "…"
	}
	return a.Pealkiri
}

func uniq(s []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range s {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

// ---------- sisukord ja otsing ----------

// SisukorraRida on akti struktuuriüksus.
type SisukorraRida struct {
	Tyyp     string `json:"tyyp"` // osa, peatykk, jagu, jaotis, paragrahv
	Nr       string `json:"nr"`
	Pealkiri string `json:"pealkiri,omitempty"`
}

// Sisukord tagastab akti struktuuri.
type Sisukord struct {
	Akt      Akt             `json:"akt"`
	Read     []SisukorraRida `json:"read"`
	Paritolu Paritolu        `json:"paritolu"`
}

var struktuuriTyybid = map[string]string{"osa": "osaPealkiri", "peatykk": "peatykkPealkiri", "jagu": "jaguPealkiri", "jaotis": "jaotisPealkiri", "alljaotis": "alljaotisPealkiri", "paragrahv": "paragrahvPealkiri"}

// AktiSisukord tagastab akti osad, peatükid ja paragrahvid koos pealkirjadega.
func (r *RT) AktiSisukord(ctx context.Context, akt, kuupaev string) (*Sisukord, error) {
	id, err := r.Lahenda(ctx, akt)
	if err != nil {
		return nil, err
	}
	a, err := r.ValiRedaktsioon(ctx, id, kuupaev)
	if err != nil {
		return nil, err
	}
	doc, vs, err := r.tekstiXML(ctx, a.ID)
	if err != nil {
		return nil, err
	}
	out := &Sisukord{Akt: *a, Paritolu: paritolu(a, vs)}
	var walk func(n *solm)
	walk = func(n *solm) {
		for _, c := range n.lapsed {
			if pk, ok := struktuuriTyybid[c.nimi]; ok {
				nr := c.kuvatavNr()
				out.Read = append(out.Read, SisukorraRida{Tyyp: c.nimi, Nr: nr, Pealkiri: textOf(c, pk)})
				if c.nimi == "paragrahv" {
					continue
				}
			}
			if c.nimi != "" {
				walk(c)
			}
		}
	}
	walk(doc)
	return out, nil
}

// Vaste on otsingutulemus aktis.
type Vaste struct {
	Viide   string `json:"viide"`
	Katkend string `json:"katkend"`
}

// OtsiVastus on aktist otsimise tulemus.
type OtsiVastus struct {
	Akt      Akt      `json:"akt"`
	Paring   string   `json:"paring"`
	Vasted   []Vaste  `json:"vasted"`
	Paritolu Paritolu `json:"paritolu"`
}

// OtsiAktist leiab lõiked/punktid, mis sisaldavad kõiki päringu sõnu
// (tõstutundetu, sõna algus sobib ka käändevormidele: "jäätmevaldaja" leiab
// ka "jäätmevaldajal").
func (r *RT) OtsiAktist(ctx context.Context, akt, paring, kuupaev string, piir int) (*OtsiVastus, error) {
	id, err := r.Lahenda(ctx, akt)
	if err != nil {
		return nil, err
	}
	a, err := r.ValiRedaktsioon(ctx, id, kuupaev)
	if err != nil {
		return nil, err
	}
	doc, vs, err := r.tekstiXML(ctx, a.ID)
	if err != nil {
		return nil, err
	}
	sonad := strings.FieldsFunc(normaliseeri(paring), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	if len(sonad) == 0 {
		return nil, viga.Uus(viga.Sisend, allikas, "", "otsingusõna puudub", nil)
	}
	if piir <= 0 {
		piir = 20
	}
	out := &OtsiVastus{Akt: *a, Paring: paring, Vasted: []Vaste{}, Paritolu: paritolu(a, vs)}
	for _, p := range doc.otsi("paragrahv") {
		pnr := nrUlaindeksiga(p, "paragrahvNr")
		for _, l := range p.koik("loige") {
			lnr := nrUlaindeksiga(l, "loigeNr")
			t := textOf(l, "sisuTekst")
			for _, ap := range l.koik("alampunkt") {
				t += " " + ap.kuvatavNr() + " " + textOf(ap, "sisuTekst")
			}
			low := normaliseeri(t)
			koik := true
			for _, s := range sonad {
				if !strings.Contains(low, s) {
					koik = false
					break
				}
			}
			if !koik {
				continue
			}
			v := "§ " + pnr
			if lnr != "" {
				v += " lg " + lnr
			}
			out.Vasted = append(out.Vasted, Vaste{Viide: v, Katkend: katkend(t, sonad[0], 240)})
			if len(out.Vasted) >= piir {
				return out, nil
			}
		}
	}
	return out, nil
}

func katkend(t, sona string, n int) string {
	low := strings.ToLower(t)
	i := strings.Index(low, sona)
	r := []rune(t)
	if i < 0 || len(r) <= n {
		if len(r) > n {
			return string(r[:n]) + "…"
		}
		return t
	}
	start := len([]rune(t[:i])) - n/3
	if start < 0 {
		start = 0
	}
	end := start + n
	if end > len(r) {
		end = len(r)
	}
	s := string(r[start:end])
	if start > 0 {
		s = "…" + s
	}
	if end < len(r) {
		s += "…"
	}
	return s
}

// ---------- redaktsioonid avalikult ----------

// RedaktsioonidVastus loetleb akti redaktsioonid.
type RedaktsioonidVastus struct {
	Akt           Akt           `json:"akt"`
	Redaktsioonid []Redaktsioon `json:"redaktsioonid"`
}

// AktiRedaktsioonid tagastab akti redaktsioonide loendi.
func (r *RT) AktiRedaktsioonid(ctx context.Context, akt string, piir int) (*RedaktsioonidVastus, error) {
	id, err := r.Lahenda(ctx, akt)
	if err != nil {
		return nil, err
	}
	a, err := r.Meta(ctx, id)
	if err != nil {
		return nil, err
	}
	reds, err := r.Redaktsioonid(ctx, a.GrupiID)
	if err != nil {
		return nil, err
	}
	if piir > 0 && len(reds) > piir {
		reds = reds[:piir]
	}
	return &RedaktsioonidVastus{Akt: *a, Redaktsioonid: reds}, nil
}

// ---------- lisad ----------

// Lisa on akti lisa (eraldi fail).
type Lisa struct {
	Nimetus string `json:"nimetus"`
	Fail    string `json:"fail"`
	Vorming string `json:"vorming"`
	URL     string `json:"url"`
}

// LisadVastus loetleb akti lisad.
type LisadVastus struct {
	Akt      Akt      `json:"akt"`
	Lisad    []Lisa   `json:"lisad"`
	Paritolu Paritolu `json:"paritolu"`
}

// AktiLisad loetleb akti redaktsiooni lisad (nt määruse lisa 2).
func (r *RT) AktiLisad(ctx context.Context, akt, kuupaev string) (*LisadVastus, error) {
	id, err := r.Lahenda(ctx, akt)
	if err != nil {
		return nil, err
	}
	a, err := r.ValiRedaktsioon(ctx, id, kuupaev)
	if err != nil {
		return nil, err
	}
	doc, vs, err := r.tekstiXML(ctx, a.ID)
	if err != nil {
		return nil, err
	}
	out := &LisadVastus{Akt: *a, Lisad: []Lisa{}, Paritolu: paritolu(a, vs)}
	for _, lv := range doc.otsi("lisaViide") {
		var fail *solm
		for _, f := range lv.otsi("fail") {
			fail = f
			break
		}
		nimetus := ""
		for _, t := range lv.otsi("tavatekst") {
			nimetus = strings.TrimSpace(t.tekstSisu())
			break
		}
		if pk := lv.laps("lisaPealkiri"); pk != nil {
			if n := strings.TrimSpace(pk.tekstSisu()); n != "" && !strings.Contains(nimetus, n) {
				nimetus = strings.TrimSpace(nimetus + " " + n)
			}
		}
		if fail == nil || fail.attr["failNimi"] == "" {
			continue
		}
		if a.lisaAadress == "" {
			return nil, viga.Struktuur(allikas, a.URL, "akti metaandmetest puudub lisaAadress")
		}
		out.Lisad = append(out.Lisad, Lisa{Nimetus: nimetus, Fail: fail.attr["failNimi"], Vorming: strings.ToLower(fail.attr["failVorming"]), URL: a.lisaAadress + url.PathEscape(fail.attr["failNimi"])})
	}
	return out, nil
}

// LisaTekstVastus on lisa tekst.
type LisaTekstVastus struct {
	Akt       Akt      `json:"akt"`
	Lisa      Lisa     `json:"lisa"`
	Tekst     string   `json:"tekst,omitempty"`
	Hoiatused []string `json:"hoiatused,omitempty"`
	Paritolu  Paritolu `json:"paritolu"`
}

var lisaNrRe = regexp.MustCompile(`(?i)^(?:lisa\s*)?(\d{1,3}|[a-z])$`)

// LisaTekst laeb akti lisa teksti. PDF-lisad teisendatakse veergude paigutust
// säilitades; tabelite numbrid tuleb tsiteerimisel PDFist üle kontrollida.
func (r *RT) LisaTekst(ctx context.Context, akt, lisa, kuupaev string) (*LisaTekstVastus, error) {
	m := lisaNrRe.FindStringSubmatch(strings.TrimSpace(lisa))
	if m == nil {
		return nil, viga.Uus(viga.Sisend, allikas, "", "lisa peab olema nt 2 või \"Lisa 2\"", nil)
	}
	l, err := r.AktiLisad(ctx, akt, kuupaev)
	if err != nil {
		return nil, err
	}
	var valitud *Lisa
	re := regexp.MustCompile(`(?i)\blisa\s*` + regexp.QuoteMeta(m[1]) + `\b`)
	for i := range l.Lisad {
		if re.MatchString(l.Lisad[i].Nimetus) || re.MatchString(strings.ReplaceAll(l.Lisad[i].Fail, "_", " ")) {
			valitud = &l.Lisad[i]
			break
		}
	}
	if valitud == nil {
		var nimed []string
		for _, x := range l.Lisad {
			nimed = append(nimed, x.Nimetus)
		}
		return nil, viga.Uus(viga.EiLeitud, allikas, l.Akt.URL, fmt.Sprintf("lisa %s puudub; aktil on lisad: %s", m[1], strings.Join(nimed, ", ")), nil)
	}
	vs, err := r.k.Tee(ctx, klient.Paring{Allikas: allikas, URL: valitud.URL, TTL: 7 * 24 * time.Hour})
	if err != nil {
		return nil, err
	}
	out := &LisaTekstVastus{Akt: l.Akt, Lisa: *valitud, Paritolu: Paritolu{Allikas: "Riigi Teataja", URL: valitud.URL, AndmeURL: vs.URL, Laaditud: vs.Laaditud, SHA256: vs.SHA256, Vahemalust: vs.Vahemalust}}
	ct := strings.ToLower(vs.SisuTyyp)
	switch {
	case strings.Contains(ct, "pdf") || valitud.Vorming == "pdf":
		t, err := tekst.PDFTabelinaTekstiks(ctx, vs.Keha)
		if err != nil {
			if err == tekst.ErrPDFTeisendajaPuudub {
				return nil, viga.Uus(viga.Soltuvus, allikas, valitud.URL, err.Error(), nil)
			}
			return nil, viga.Uus(viga.StruktuurMuutunud, allikas, valitud.URL, "PDF teisendamine ebaõnnestus", err)
		}
		if len([]rune(strings.TrimSpace(t))) < 20 {
			out.Hoiatused = append(out.Hoiatused, "PDFist ei saadud teksti (skannitud pilt?); ava lisa lingi kaudu")
		}
		out.Tekst = t
		out.Hoiatused = append(out.Hoiatused, "LISA ON PDF: tabelite numbrid (komakohad, ühikud) võivad tekstiks teisendamisel moonduda – kontrolli tsiteeritavad väärtused PDFist")
	case strings.Contains(ct, "html"):
		t, err := tekst.HTMLTekstiks(vs.Keha)
		if err != nil {
			return nil, viga.Uus(viga.StruktuurMuutunud, allikas, valitud.URL, "HTML ei ole loetav", err)
		}
		out.Tekst = t
	default:
		out.Hoiatused = append(out.Hoiatused, fmt.Sprintf("lisa vorming %q ei ole tekstina loetav; ava lisa lingi kaudu", valitud.Vorming))
	}
	return out, nil
}
