package viga

import (
	"errors"
	"fmt"
	"net/url"
	"runtime"
	"strings"
)

// Hoidla on tööriista lähtekoodi hoidla (probleemid ja parandused).
const Hoidla = "https://github.com/Ker102/oigusallikad"

// Soovitus tagastab kasutajale suunatud juhise vea korral, mille põhjus on
// tööriistas või allika muutuses (mitte puuduvas dokumendis). Tühi, kui
// juhist pole vaja (nt "ei leitud", mille korral agent kasutab muud allikat).
func Soovitus(err error) string {
	if err == nil {
		return ""
	}
	var v *Viga
	liik := Liik("")
	if errors.As(err, &v) {
		liik = v.Liik
	}
	switch liik {
	case EiLeitud, Sisend:
		return ""
	case Kattesaamatu:
		return "Allikas ei ole hetkel kättesaadav. Proovi mõne aja pärast uuesti; kui viga kordub päevi, käivita `oigusallikad kontroll -t` ja vaata allpool olevaid samme."
	case Soltuvus:
		return "Paigalda puuduv abiprogramm (README: Paigaldamine)."
	}
	// StruktuurMuutunud, Blokeeritud ja ootamatud vead: tööriist vajab tõenäoliselt parandust.
	var b strings.Builder
	switch liik {
	case StruktuurMuutunud:
		b.WriteString("Allika vorming on tõenäoliselt muutunud ja tööriist vajab uuendamist. ")
	case Blokeeritud:
		b.WriteString("Allikas blokeerib nüüd automaatpäringud; tööriist ei ürita blokeeringust mööda minna, vaja on uut ametlikku andmeteed. ")
	default:
		b.WriteString("Tööriist ei töötanud ootuspäraselt. ")
	}
	b.WriteString("Soovita kasutajale:\n")
	b.WriteString("1) uuenda tööriista: go install github.com/Ker102/oigusallikad/cmd/oigusallikad@latest\n")
	b.WriteString("2) vaata, kas viga on juba teada või parandatud: " + Hoidla + "/issues\n")
	b.WriteString("3) käivita `oigusallikad kontroll -t`, et näha, milline allikas on katki\n")
	b.WriteString("4) kui parandust pole: `oigusallikad veateade` koostab GitHubi probleemi (issue) eeltäidetud kirjelduse; GitHubi kasutajaga saab saata ka paranduse (pull request) – juhend README-s (\"Kui allikas muutub\")\n")
	b.WriteString("Seni kasuta sama dokumendi jaoks muud allikat (nt brauser või üldine veebikaabits) ja ütle kasutajale, kust tekst pärineb.")
	return b.String()
}

// VeateateURL koostab GitHubi uue probleemi lingi eeltäidetud pealkirja ja
// kirjeldusega.
func VeateateURL(versioon, pealkiri, kirjeldus string) string {
	body := fmt.Sprintf("**Versioon:** %s\n**Süsteem:** %s/%s\n\n%s\n\n**Mida ootasin:**\n\n**Mis juhtus:**\n", versioon, runtime.GOOS, runtime.GOARCH, kirjeldus)
	q := url.Values{}
	q.Set("title", pealkiri)
	q.Set("body", body)
	q.Set("labels", "allikas-muutunud")
	return Hoidla + "/issues/new?" + q.Encode()
}
