// Package mcpserver avaldab õigusallikate tööriistad MCP serverina.
package mcpserver

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Ker102/oigusallikad/internal/eurlex"
	"github.com/Ker102/oigusallikad/internal/kohtulahendid"
	"github.com/Ker102/oigusallikad/internal/kontroll"
	"github.com/Ker102/oigusallikad/internal/rt"
	"github.com/Ker102/oigusallikad/internal/tekst"
	"github.com/Ker102/oigusallikad/internal/viga"
)

// Allikad koondab kliendid.
type Allikad struct {
	RT     *rt.RT
	Kohtud *kohtulahendid.Klient
	EL     *eurlex.Klient
}

type satteSisend struct {
	Akt       string `json:"akt" jsonschema:"seaduse lühend (THS, JäätS, HMS), Riigi Teataja link või akti ID; määrustel link või ID"`
	Paragrahv string `json:"paragrahv" jsonschema:"paragrahvi number, nt 98 või 88^1"`
	Loige     string `json:"loige,omitempty" jsonschema:"lõike number (valikuline)"`
	Punkt     string `json:"punkt,omitempty" jsonschema:"punkti number (valikuline, vajab lõiget)"`
	Kuupaev   string `json:"kuupaev,omitempty" jsonschema:"redaktsioon, mis kehtis sel kuupäeval (AAAA-KK-PP); vaikimisi täna"`
}

type aktiSisend struct {
	Akt     string `json:"akt" jsonschema:"seaduse lühend, Riigi Teataja link või akti ID"`
	Kuupaev string `json:"kuupaev,omitempty" jsonschema:"redaktsiooni kuupäev AAAA-KK-PP; vaikimisi täna"`
}

type aktiOtsingSisend struct {
	Akt     string `json:"akt" jsonschema:"seaduse lühend, Riigi Teataja link või akti ID"`
	Paring  string `json:"paring" jsonschema:"sõnad, mis peavad lõikes esinema (sõna algus sobib ka käändevormile)"`
	Kuupaev string `json:"kuupaev,omitempty" jsonschema:"redaktsiooni kuupäev AAAA-KK-PP"`
	Piir    int    `json:"piir,omitempty" jsonschema:"tulemuste arv (vaikimisi 20)"`
}

type lisaSisend struct {
	Akt     string `json:"akt" jsonschema:"seaduse lühend, Riigi Teataja link või akti ID"`
	Lisa    string `json:"lisa,omitempty" jsonschema:"lisa number (nt 2); tühi = loetle lisad"`
	Kuupaev string `json:"kuupaev,omitempty" jsonschema:"redaktsiooni kuupäev AAAA-KK-PP"`
}

type lisaVastus struct {
	Loend *rt.LisadVastus     `json:"loend,omitempty"`
	Lisa  *rt.LisaTekstVastus `json:"lisa,omitempty"`
}

type redSisend struct {
	Akt  string `json:"akt" jsonschema:"seaduse lühend, Riigi Teataja link või akti ID"`
	Piir int    `json:"piir,omitempty" jsonschema:"mitu viimast redaktsiooni (vaikimisi kõik)"`
}

type lahendOtsiSisend struct {
	Tekst  string `json:"tekst,omitempty" jsonschema:"täistekstiotsing lahendite sisust"`
	AsjaNr string `json:"asja_nr,omitempty" jsonschema:"kohtuasja number, nt 3-22-1501"`
	Aste   string `json:"aste,omitempty" jsonschema:"riigikohus, ringkonnakohus või esimene"`
	Alates string `json:"alates,omitempty" jsonschema:"lahendi kuupäev alates AAAA-KK-PP"`
	Kuni   string `json:"kuni,omitempty" jsonschema:"lahendi kuupäev kuni AAAA-KK-PP"`
	Piir   int    `json:"piir,omitempty" jsonschema:"tulemuste arv (vaikimisi 20)"`
}

type lahendSisend struct {
	AsjaNr   string `json:"asja_nr" jsonschema:"lahendi number koos dokumendi numbriga, nt 3-15-443/54"`
	Punktid  string `json:"punktid,omitempty" jsonschema:"punktid, nt 12,13 või 20-22; soovitatav, säästab konteksti"`
	Ulevaade bool   `json:"ulevaade,omitempty" jsonschema:"iga punkti esimesed ~160 märki, et leida õiged punktid"`
	MaxMarke int    `json:"max_marke,omitempty" jsonschema:"täisteksti piir märkides (vaikimisi 20000)"`
}

type elSisend struct {
	Viide       string   `json:"viide" jsonschema:"CELEX (32008L0098, 62022CJ0626) või kohtuasi (C-626/22)"`
	Liik        string   `json:"liik,omitempty" jsonschema:"kohtuasja puhul: otsus (vaikimisi), ettepanek (kohtujurist), maarus"`
	Keel        string   `json:"keel,omitempty" jsonschema:"keelekood, vaikimisi ET"`
	Artiklid    []string `json:"artiklid,omitempty" jsonschema:"õigusakti artiklid, nt [\"15\",\"14\"]"`
	Punktid     string   `json:"punktid,omitempty" jsonschema:"kohtulahendi punktid, nt 94 või 35-39"`
	Allmarkused string   `json:"allmarkused,omitempty" jsonschema:"allmärkused, nt 49 (kohtujuristi ettepanekutes on sisu sageli allmärkustes)"`
	MaxMarke    int      `json:"max_marke,omitempty" jsonschema:"täisteksti piir (vaikimisi 20000)"`
}

type tyhi struct{}

// vigaTekstiks lisab vea liigi, et agent eristaks "ei leitud" ja "struktuur muutus".
func vigaTekstiks(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	var v *viga.Viga
	if errors.As(err, &v) {
		switch v.Liik {
		case viga.StruktuurMuutunud:
			msg += " — ALLIKA STRUKTUUR ON MUUTUNUD: ära järelda, et dokumenti pole."
		case viga.Blokeeritud:
			msg += " — allikas blokeeris automaatpäringu: ära järelda, et dokumenti pole."
		}
	}
	if s := viga.Soovitus(err); s != "" {
		msg += "\n\n" + s
	}
	return errors.New(msg)
}

// Uus loob serveri.
func Uus(versioon string, a Allikad) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "oigusallikad", Version: versioon}, &mcp.ServerOptions{
		Instructions: "Eesti ja ELi õigusallikad algallikast: Riigi Teataja (seadused, määrused, kohtulahendid), Riigikohtu lahendite andmebaas ja EUR-Lex (Cellar). " +
			"Küsi võimalikult kitsalt (paragrahv/lõige, lahendi punktid, artiklid), et säästa konteksti. Iga vastus sisaldab päritolu (URL, laadimisaeg, SHA-256). " +
			"Vea liik 'ei_leitud' tähendab, et allikas vastas korrektselt – kasuta vajadusel muud allikat. " +
			"Vea liik 'struktuur_muutunud' või 'blokeeritud' EI tähenda, et dokumenti pole: tööriist vajab tõenäoliselt uuendust – järgi veas olevaid samme ja teavita kasutajat (uuendamine, teadaolevad probleemid, veateade/parandus GitHubis).",
	})

	mcp.AddTool(s, &mcp.Tool{Name: "rt_satte_tekst", Description: "Riigi Teataja: seaduse või määruse paragrahvi (lõike, punkti) tekst ametlikust XMList, soovi korral antud kuupäeval kehtinud redaktsioonist."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in satteSisend) (*mcp.CallToolResult, *rt.SatteVastus, error) {
			v, err := a.RT.Satted(ctx, in.Akt, in.Paragrahv, in.Loige, in.Punkt, in.Kuupaev)
			return nil, v, vigaTekstiks(err)
		})
	mcp.AddTool(s, &mcp.Tool{Name: "rt_akti_sisukord", Description: "Riigi Teataja: akti osad, peatükid ja paragrahvid koos pealkirjadega (õige paragrahvi leidmiseks)."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in aktiSisend) (*mcp.CallToolResult, *rt.Sisukord, error) {
			v, err := a.RT.AktiSisukord(ctx, in.Akt, in.Kuupaev)
			return nil, v, vigaTekstiks(err)
		})
	mcp.AddTool(s, &mcp.Tool{Name: "rt_otsi_aktist", Description: "Riigi Teataja: leia akti lõiked, mis sisaldavad kõiki antud sõnu (tõstutundetu; sõna algus sobib käändevormidele)."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in aktiOtsingSisend) (*mcp.CallToolResult, *rt.OtsiVastus, error) {
			v, err := a.RT.OtsiAktist(ctx, in.Akt, in.Paring, in.Kuupaev, in.Piir)
			return nil, v, vigaTekstiks(err)
		})
	mcp.AddTool(s, &mcp.Tool{Name: "rt_redaktsioonid", Description: "Riigi Teataja: akti tervikteksti redaktsioonid ja nende kehtivusajad."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in redSisend) (*mcp.CallToolResult, *rt.RedaktsioonidVastus, error) {
			v, err := a.RT.AktiRedaktsioonid(ctx, in.Akt, in.Piir)
			return nil, v, vigaTekstiks(err)
		})
	mcp.AddTool(s, &mcp.Tool{Name: "rt_akti_lisad", Description: "Riigi Teataja: akti lisad (nt määruse lisa 2 piirväärtuste tabel). Ilma lisa numbrita loetleb lisad; numbriga tagastab lisa teksti. PDF-tabelite numbrid kontrolli PDFist."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in lisaSisend) (*mcp.CallToolResult, lisaVastus, error) {
			if in.Lisa == "" {
				v, err := a.RT.AktiLisad(ctx, in.Akt, in.Kuupaev)
				return nil, lisaVastus{Loend: v}, vigaTekstiks(err)
			}
			v, err := a.RT.LisaTekst(ctx, in.Akt, in.Lisa, in.Kuupaev)
			return nil, lisaVastus{Lisa: v}, vigaTekstiks(err)
		})
	mcp.AddTool(s, &mcp.Tool{Name: "kohtulahendid_otsi", Description: "Riigi Teataja: Eesti kohtulahendite otsing teksti või kohtuasja numbri järgi (kõik astmed)."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in lahendOtsiSisend) (*mcp.CallToolResult, *kohtulahendid.OtsinguVastus, error) {
			piir := in.Piir
			if piir <= 0 {
				piir = 20
			}
			v, err := a.Kohtud.Otsi(ctx, kohtulahendid.Valikud{Tekst: in.Tekst, AsjaNr: in.AsjaNr, Aste: in.Aste, Alates: in.Alates, Kuni: in.Kuni, Piir: piir})
			return nil, v, vigaTekstiks(err)
		})
	mcp.AddTool(s, &mcp.Tool{Name: "kohtulahend", Description: "Eesti kohtulahendi tekst: Riigikohtu lahendid Riigikohtu andmebaasist, muud Riigi Teatajast. Küsi punkte (punktid) või ülevaadet, mitte kogu teksti."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in lahendSisend) (*mcp.CallToolResult, *kohtulahendid.TekstiVastus, error) {
			var p []int
			if in.Punktid != "" {
				var err error
				if p, err = tekst.ParsiPunktid(in.Punktid); err != nil {
					return nil, nil, err
				}
			}
			v, err := a.Kohtud.Tekst(ctx, kohtulahendid.TekstiValikud{AsjaNr: in.AsjaNr, Punktid: p, Ulevaade: in.Ulevaade, MaxMarke: in.MaxMarke})
			return nil, v, vigaTekstiks(err)
		})
	mcp.AddTool(s, &mcp.Tool{Name: "eurlex_dokument", Description: "EUR-Lex (Cellar): ELi õigusakti artiklid või Euroopa Kohtu lahendi punktid ametlikus keeleversioonis (vaikimisi eesti keel)."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in elSisend) (*mcp.CallToolResult, *eurlex.Vastus, error) {
			var p []int
			if in.Punktid != "" {
				var err error
				if p, err = tekst.ParsiPunktid(in.Punktid); err != nil {
					return nil, nil, err
				}
			}
			var am []int
			if in.Allmarkused != "" {
				var err error
				if am, err = tekst.ParsiPunktid(in.Allmarkused); err != nil {
					return nil, nil, err
				}
			}
			v, err := a.EL.Dokument(ctx, eurlex.Valikud{Viide: in.Viide, Liik: in.Liik, Keel: in.Keel, Artiklid: in.Artiklid, Punktid: p, Allmarkused: am, MaxMarke: in.MaxMarke})
			return nil, v, vigaTekstiks(err)
		})
	mcp.AddTool(s, &mcp.Tool{Name: "allikate_kontroll", Description: "Kontrollib, kas kõik allikad töötavad ja annavad oodatud sisu (teadaolevad dokumendid). Kasuta, kui tööriist annab ootamatuid vigu."},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ tyhi) (*mcp.CallToolResult, kontroll.Aruanne, error) {
			return nil, kontroll.Kaivita(ctx, a.RT, a.Kohtud, a.EL), nil
		})
	return s
}

// Kaivita käivitab serveri stdio kaudu.
func Kaivita(ctx context.Context, versioon string, a Allikad) error {
	return Uus(versioon, a).Run(ctx, &mcp.StdioTransport{})
}
