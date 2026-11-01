// Käsk oigusallikad laeb Eesti ja ELi õigusallikaid algallikast: Riigi
// Teataja (aktid ja kohtulahendid), Riigikohtu lahendite andmebaas ja
// EUR-Lex (Cellar). Töötab käsurealt ja MCP serverina.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/Ker102/oigusallikad/internal/eurlex"
	"github.com/Ker102/oigusallikad/internal/klient"
	"github.com/Ker102/oigusallikad/internal/kohtulahendid"
	"github.com/Ker102/oigusallikad/internal/kontroll"
	"github.com/Ker102/oigusallikad/internal/mcpserver"
	"github.com/Ker102/oigusallikad/internal/rt"
	"github.com/Ker102/oigusallikad/internal/tekst"
	"github.com/Ker102/oigusallikad/internal/viga"
)

var versioon = "0.1.0"

const kasutus = `oigusallikad – Eesti ja ELi õigusallikad algallikast

Kasutus:
  oigusallikad <käsk> [valikud] argumendid

Riigi Teataja (seadused ja määrused):
  sate AKT PARAGRAHV            paragrahvi tekst (nt: sate THS 98 -lg 3)
  sisukord AKT                  akti peatükid ja paragrahvid
  otsi-aktist AKT SÕNAD         lõiked, kus esinevad kõik sõnad
  redaktsioonid AKT             tervikteksti redaktsioonid
  lisad AKT                     akti lisad (failid) ja nende lingid
  lisa AKT NR                   lisa tekst (nt: lisa 101072026014 2)

Kohtulahendid:
  lahendid TEKST                otsi lahendite tekstist
  lahendid -asi 3-22-1501       otsi kohtuasja numbri järgi
  lahend ASJA_NR                lahendi tekst (nt: lahend 3-15-443/54 -punktid 12,13)

EUR-Lex:
  eurlex VIIDE                  ELi akt või Euroopa Kohtu lahend
                                (nt: eurlex 32008L0098 -artikkel 15; eurlex C-626/22 -punktid 94)

Muu:
  kontroll                      kontrolli, kas kõik allikad töötavad (kanaarilinnud)
  veateade                      käivita kontroll ja koosta GitHubi probleemi (issue) link
  mcp                           käivita MCP server (stdio)
  versioon

Valikud:
  -t                  inimloetav tekst JSONi asemel
  -kp AAAA-KK-PP      akti redaktsioon, mis kehtis sel kuupäeval
  -lg N, -p N         lõige ja punkt (sate)
  -punktid 12,13      lahendi punktid (lahend, eurlex)
  -ylevaade           lahendi punktide algused (lahend)
  -artikkel 15,16     ELi akti artiklid (eurlex)
  -allmarkused 49     allmärkused (eurlex; nt kohtujuristi ettepanekud)
  -liik otsus|ettepanek|maarus   Euroopa Kohtu dokumendi liik (eurlex C-…)
  -keel ET            ELi dokumendi keel
  -aste riigikohus|ringkonnakohus|esimene   (lahendid)
  -piir N             tulemuste arv
  -max N              täisteksti märkide piir
  -varske             ära kasuta vahemälu
  -vahemalu KAUST     vahemälu kaust ("-" lülitab välja)

Väljumiskoodid: 0 korras; 1 muu viga; 2 vale kasutus; 3 ei leitud;
                4 allika struktuur muutunud; 5 allikas blokeeris; 6 allikas kättesaamatu;
                7 kohalik sõltuvus puudub.
`

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, out, errw io.Writer) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "abi" {
		fmt.Fprint(out, kasutus)
		return 0
	}
	cmd, rest := args[0], args[1:]
	if cmd == "versioon" || cmd == "--version" {
		fmt.Fprintln(out, "oigusallikad", versioon)
		return 0
	}
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(errw)
	var (
		txt      = fs.Bool("t", false, "inimloetav väljund")
		kp       = fs.String("kp", "", "redaktsiooni kuupäev")
		lg       = fs.String("lg", "", "lõige")
		pkt      = fs.String("p", "", "punkt")
		punktid  = fs.String("punktid", "", "lahendi punktid")
		ylevaade = fs.Bool("ylevaade", false, "punktide ülevaade")
		artikkel = fs.String("artikkel", "", "artiklid")
		allm     = fs.String("allmarkused", "", "allmärkused")
		liik     = fs.String("liik", "", "Euroopa Kohtu dokumendi liik")
		keel     = fs.String("keel", "ET", "keel")
		aste     = fs.String("aste", "", "kohtuaste")
		asi      = fs.String("asi", "", "kohtuasja number")
		alates   = fs.String("alates", "", "alates kuupäevast")
		kuni     = fs.String("kuni", "", "kuni kuupäevani")
		piir     = fs.Int("piir", 20, "tulemuste arv")
		max      = fs.Int("max", 0, "märkide piir")
		varske   = fs.Bool("varske", false, "ignoreeri vahemälu")
		vahemalu = fs.String("vahemalu", "", "vahemälu kaust")
	)
	// luba valikuid ka pärast positsioonilisi argumente
	var pos []string
	for len(rest) > 0 {
		if err := fs.Parse(rest); err != nil {
			return 2
		}
		rest = fs.Args()
		if len(rest) > 0 {
			pos = append(pos, rest[0])
			rest = rest[1:]
		}
	}

	klient.Versioon = versioon
	k := klient.Uus(klient.Valikud{Vahemalu: *vahemalu, Varske: *varske})
	a := mcpserver.Allikad{RT: rt.Uus(k), Kohtud: kohtulahendid.Uus(k), EL: eurlex.Uus(k)}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	need := func(n int, mis string) bool {
		if len(pos) < n {
			fmt.Fprintf(errw, "käsk %q vajab: %s\n", cmd, mis)
			return false
		}
		return true
	}
	var res any
	var err error
	switch cmd {
	case "mcp":
		if err := mcpserver.Kaivita(ctx, versioon, a); err != nil {
			fmt.Fprintln(errw, "viga:", err)
			return 1
		}
		return 0
	case "sate":
		if !need(2, "AKT PARAGRAHV") {
			return 2
		}
		res, err = a.RT.Satted(ctx, pos[0], pos[1], *lg, *pkt, *kp)
	case "sisukord":
		if !need(1, "AKT") {
			return 2
		}
		res, err = a.RT.AktiSisukord(ctx, pos[0], *kp)
	case "otsi-aktist":
		if !need(2, "AKT SÕNAD") {
			return 2
		}
		res, err = a.RT.OtsiAktist(ctx, pos[0], strings.Join(pos[1:], " "), *kp, *piir)
	case "redaktsioonid":
		if !need(1, "AKT") {
			return 2
		}
		res, err = a.RT.AktiRedaktsioonid(ctx, pos[0], *piir)
	case "lisad":
		if !need(1, "AKT") {
			return 2
		}
		res, err = a.RT.AktiLisad(ctx, pos[0], *kp)
	case "lisa":
		if !need(2, "AKT NR") {
			return 2
		}
		res, err = a.RT.LisaTekst(ctx, pos[0], strings.Join(pos[1:], " "), *kp)
	case "veateade":
		ar := kontroll.Kaivita(ctx, a.RT, a.Kohtud, a.EL)
		var b strings.Builder
		var vigased []string
		for _, t := range ar.Tulemused {
			if !t.Korras {
				vigased = append(vigased, t.Nimi)
				fmt.Fprintf(&b, "- **%s** (%s): `%s` %s\n", t.Nimi, t.Allikas, t.Liik, t.Teade)
			}
		}
		if len(vigased) == 0 {
			fmt.Fprintln(out, "Kõik kontrollid on korras. Kui tööriist annab siiski vale tulemuse, kirjelda probleemi siin:")
			fmt.Fprintln(out, viga.VeateateURL(versioon, "Vale tulemus: ", "**Käsk ja sisend:**\n\n**Väljund:**"))
			return 0
		}
		pealkiri := "Allikas muutunud: " + strings.Join(vigased, "; ")
		if len(pealkiri) > 120 {
			pealkiri = pealkiri[:117] + "..."
		}
		fmt.Fprintf(out, "Ebaõnnestunud kontrollid:\n%s\nAva link, et saata probleem (vajab GitHubi kasutajat):\n%s\n\nParanduse saatmise juhend: %s#kui-allikas-muutub\n",
			b.String(), viga.VeateateURL(versioon, pealkiri, "**Ebaõnnestunud kontrollid:**\n"+b.String()), viga.Hoidla)
		return 4
	case "lahendid":
		res, err = a.Kohtud.Otsi(ctx, kohtulahendid.Valikud{Tekst: strings.Join(pos, " "), AsjaNr: *asi, Aste: *aste, Alates: *alates, Kuni: *kuni, Piir: *piir})
	case "lahend":
		if !need(1, "ASJA_NR") {
			return 2
		}
		var p []int
		if *punktid != "" {
			if p, err = tekst.ParsiPunktid(*punktid); err != nil {
				fmt.Fprintln(errw, "viga:", err)
				return 2
			}
		}
		res, err = a.Kohtud.Tekst(ctx, kohtulahendid.TekstiValikud{AsjaNr: pos[0], Punktid: p, Ulevaade: *ylevaade, MaxMarke: *max})
	case "eurlex":
		if !need(1, "VIIDE") {
			return 2
		}
		var p []int
		if *punktid != "" {
			if p, err = tekst.ParsiPunktid(*punktid); err != nil {
				fmt.Fprintln(errw, "viga:", err)
				return 2
			}
		}
		var art []string
		if *artikkel != "" {
			art = strings.Split(*artikkel, ",")
		}
		var am []int
		if *allm != "" {
			if am, err = tekst.ParsiPunktid(*allm); err != nil {
				fmt.Fprintln(errw, "viga:", err)
				return 2
			}
		}
		res, err = a.EL.Dokument(ctx, eurlex.Valikud{Viide: pos[0], Liik: *liik, Keel: *keel, Artiklid: art, Punktid: p, Allmarkused: am, MaxMarke: *max})
	case "kontroll":
		ar := kontroll.Kaivita(ctx, a.RT, a.Kohtud, a.EL)
		if *txt {
			for _, t := range ar.Tulemused {
				olek := "OK  "
				if !t.Korras {
					olek = "VIGA"
				}
				fmt.Fprintf(out, "%s  %-55s %8s  %s %s\n", olek, t.Nimi, t.Kestus, t.Liik, t.Teade)
			}
		} else {
			kirjutaJSON(out, ar)
		}
		if !ar.Korras {
			fmt.Fprintf(errw, "\nVähemalt üks allikas ei tööta ootuspäraselt. Uuenda tööriista (go install github.com/Ker102/oigusallikad/cmd/oigusallikad@latest),\nvaata teadaolevaid probleeme (%s/issues) või käivita `oigusallikad veateade`.\n", viga.Hoidla)
			return 4
		}
		return 0
	default:
		fmt.Fprintf(errw, "tundmatu käsk %q\n\n%s", cmd, kasutus)
		return 2
	}
	if err != nil {
		fmt.Fprintln(errw, "viga:", err)
		if sv := viga.Soovitus(err); sv != "" {
			fmt.Fprintln(errw, "\n"+sv)
		}
		return kood(err)
	}
	if *txt {
		prindiTekst(out, res)
	} else {
		kirjutaJSON(out, res)
	}
	return 0
}

func kood(err error) int {
	switch viga.LiikVeast(err) {
	case viga.EiLeitud:
		return 3
	case viga.StruktuurMuutunud:
		return 4
	case viga.Blokeeritud:
		return 5
	case viga.Kattesaamatu:
		return 6
	case viga.Soltuvus:
		return 7
	case viga.Sisend:
		return 2
	}
	if errors.Is(err, context.Canceled) {
		return 1
	}
	return 1
}

func kirjutaJSON(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func prindiTekst(w io.Writer, res any) {
	switch v := res.(type) {
	case *rt.SatteVastus:
		fmt.Fprintf(w, "%s (%s, redaktsioon kehtiv %s–%s)\n\n%s\n", v.Viide, v.Akt.Pealkiri, v.Akt.KehtivAlates, v.Akt.KehtivKuni, v.Tekst)
		if len(v.Muudetud) > 0 {
			fmt.Fprintf(w, "\nMuudetud: %s\n", strings.Join(v.Muudetud, " "))
		}
		fmt.Fprintf(w, "\nAllikas: %s\n", v.Paritolu.URL)
	case *rt.Sisukord:
		fmt.Fprintf(w, "%s (kehtiv %s–%s)\n", v.Akt.Pealkiri, v.Akt.KehtivAlates, v.Akt.KehtivKuni)
		for _, r := range v.Read {
			ind := "  "
			if r.Tyyp == "paragrahv" {
				ind = "    "
			}
			fmt.Fprintf(w, "%s%s %s\n", ind, r.Nr, r.Pealkiri)
		}
	case *rt.OtsiVastus:
		fmt.Fprintf(w, "%s: %d vastet päringule %q\n", v.Akt.Pealkiri, len(v.Vasted), v.Paring)
		for _, x := range v.Vasted {
			fmt.Fprintf(w, "  %-14s %s\n", x.Viide, x.Katkend)
		}
	case *rt.RedaktsioonidVastus:
		fmt.Fprintln(w, v.Akt.Pealkiri)
		for _, r := range v.Redaktsioonid {
			fmt.Fprintf(w, "  %d  %s – %s\n", r.ID, r.KehtivAlates, r.KehtivKuni)
		}
	case *rt.LisadVastus:
		fmt.Fprintln(w, v.Akt.Pealkiri)
		for _, l := range v.Lisad {
			fmt.Fprintf(w, "  %-12s %-6s %s\n", l.Nimetus, l.Vorming, l.URL)
		}
	case *rt.LisaTekstVastus:
		fmt.Fprintf(w, "%s – %s\n\n%s\n", v.Akt.Pealkiri, v.Lisa.Nimetus, v.Tekst)
		for _, h := range v.Hoiatused {
			fmt.Fprintln(w, "HOIATUS:", h)
		}
		fmt.Fprintf(w, "Allikas: %s\n", v.Lisa.URL)
	case *kohtulahendid.OtsinguVastus:
		fmt.Fprintf(w, "%d tulemust (%q)\n", v.Kokku, v.Paring)
		for _, l := range v.Tulemused {
			fmt.Fprintf(w, "  %-16s %s  %s  %s\n", l.AsjaNr, l.Kuupaev, l.Kohus, strings.Join(l.Kategooriad, "; "))
		}
		if v.Markus != "" {
			fmt.Fprintln(w, "  ("+v.Markus+")")
		}
	case *kohtulahendid.TekstiVastus:
		fmt.Fprintf(w, "%s %s %s — punkte %d\n\n", v.AsjaNr, v.Kohus, v.Kuupaev, v.PunkteKokku)
		for _, p := range v.Punktid {
			fmt.Fprintf(w, "%d. %s\n\n", p.Nr, p.Tekst)
		}
		if v.Tekst != "" {
			fmt.Fprintln(w, v.Tekst)
		}
		for _, h := range v.Hoiatused {
			fmt.Fprintln(w, "HOIATUS:", h)
		}
		fmt.Fprintf(w, "Allikas: %s\n", v.Paritolu.URL)
	case *eurlex.Vastus:
		fmt.Fprintf(w, "%s (%s)\n%s\n\n", v.CELEX, v.Keel, v.Pealkiri)
		for _, o := range v.Osad {
			fmt.Fprintf(w, "[%s] %s\n\n", o.Viide, o.Tekst)
		}
		if v.Tekst != "" {
			fmt.Fprintln(w, v.Tekst)
		}
		for _, h := range v.Hoiatused {
			fmt.Fprintln(w, "HOIATUS:", h)
		}
		fmt.Fprintf(w, "Allikas: %s\n", v.Paritolu.URL)
	default:
		kirjutaJSON(w, res)
	}
}
