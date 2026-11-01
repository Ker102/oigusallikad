package eurlex

import (
	"strings"
	"testing"
)

func TestCELEXKohtuasjast(t *testing.T) {
	cases := []struct{ asi, liik, want string }{
		{"C-626/22", "", "62022CJ0626"},
		{"C‑487/17", "otsus", "62017CJ0487"},
		{"C-730/23", "ettepanek", "62023CC0730"},
		{"C-129/96", "", "61996CJ0129"},
		{"T-12/19", "", "62019TJ0012"},
	}
	for _, c := range cases {
		got, err := CELEXKohtuasjast(c.asi, c.liik)
		if err != nil || got != c.want {
			t.Errorf("%s/%s: %s %v (ootasin %s)", c.asi, c.liik, got, err, c.want)
		}
	}
	if _, err := CELEXKohtuasjast("T-12/19", "ettepanek"); err == nil {
		t.Error("Üldkohtu asjas pole kohtujuristi ettepanekut")
	}
}

// Vanem/keskmine Cellari XHTML: <p class="coj-count" id="pointN">
const xhtmlOtsus = `<html><body>
<table><tr><td><p class="coj-count" id="point93">93</p></td><td><p class="coj-normal">Eelnev analüüs.</p></td></tr></table>
<table><tr><td><p class="coj-count" id="point94">94</p></td><td><p class="coj-normal">Käitaja peab <span>pidevalt</span> hindama.</p></td></tr></table>
<p class="coj-sum-title-1">Kohtukulud</p><p>95 Kuna menetlus …</p>
</body></html>`

// Uuem HTML (esialgsed tõlked): <A NAME="pointN">N.</A> ja allmärkused NAME="FootnoteN".
const htmlEttepanek = `<HTML><BODY>
<P class="C01PointAltN">
<A NAME="point1">1.</A>&nbsp;&nbsp;&nbsp;Poola Vabariik palub(<A HREF="#Footnote2" NAME="Footref2">2</A>) tuvastada.</P>
<P class="C01PointAltN">
<A NAME="point2">2.</A>&nbsp;&nbsp;Teine punkt.</P>
<HR>
<P class="Cfootnotetext">
<A HREF="#Footref2" NAME="Footnote2">2</A>      Määrus jäätmesaadetiste kohta.</P>
<HR>
<P class="Cfootnotetext">
<A HREF="#Footref49" NAME="Footnote49">49</A>      Vt direktiivi 2008/98 artikli 15 lõige 2.</P>
</BODY></HTML>`

func TestKohtuotsusePunktid(t *testing.T) {
	p := kohtuotsusePunktid([]byte(xhtmlOtsus))
	if p[94] != "Käitaja peab pidevalt hindama." {
		t.Errorf("p 94: %q", p[94])
	}
	if strings.Contains(p[94], "Kohtukulud") {
		t.Error("viimasesse punkti jäi kohtukulude osa")
	}
	q := kohtuotsusePunktid([]byte(htmlEttepanek))
	if q[1] != "Poola Vabariik palub(2) tuvastada." || q[2] != "Teine punkt." {
		t.Errorf("uus vorming: %q / %q", q[1], q[2])
	}
	am := allmarkused([]byte(htmlEttepanek))
	if am[49] != "Vt direktiivi 2008/98 artikli 15 lõige 2." || am[2] != "Määrus jäätmesaadetiste kohta." {
		t.Errorf("allmärkused: %v", am)
	}
}

func TestArtiklid(t *testing.T) {
	tekst := "Artikkel 14\nKulud\n1. Kulud kannab tekitaja.\nArtikkel 15\nVastutus\n2. Eeltöötlemiseks edastamine ei vabasta.\nI LISA\nKÕRVALDAMISTOIMINGUD"
	a := artiklidTekstist(tekst)
	if !strings.Contains(a["15"], "Eeltöötlemiseks") || strings.Contains(a["15"], "LISA") {
		t.Errorf("art 15: %q", a["15"])
	}
	if got := strings.Join(ArtikliteLoend(a), ","); got != "14,15" {
		t.Errorf("loend: %s", got)
	}
}
