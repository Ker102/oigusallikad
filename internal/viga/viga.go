// Package viga määrab vealiigid, mille abil eristatakse "allikas ütles, et
// dokumenti pole" olukorrast "allikas muutis struktuuri" või "allikas
// blokeeris päringu". Tühja tulemust ei tagastata kunagi vaikimisi edukana,
// kui tegelik põhjus on viga.
package viga

import (
	"errors"
	"fmt"
)

// Liik on vea liik.
type Liik string

const (
	// EiLeitud: allikas vastas korrektselt, et sellist dokumenti/sätet ei ole.
	EiLeitud Liik = "ei_leitud"
	// Blokeeritud: allikas nõuab inimkontrolli (nt Cloudflare) või keelas päringu.
	Blokeeritud Liik = "blokeeritud"
	// StruktuurMuutunud: vastus ei vasta oodatud kujule – tööriista tuleb uuendada.
	StruktuurMuutunud Liik = "struktuur_muutunud"
	// Kattesaamatu: võrguviga, ajalõpp või serveri viga (5xx).
	Kattesaamatu Liik = "kattesaamatu"
	// Sisend: vigane sisend (nt vale CELEX number).
	Sisend Liik = "vigane_sisend"
	// Sõltuvus: kohalik abiprogramm (nt pdftotext) puudub.
	Soltuvus Liik = "soltuvus_puudub"
)

// Viga on tööriista viga koos allika ja URLiga.
type Viga struct {
	Liik    Liik
	Allikas string
	URL     string
	Teade   string
	Pohjus  error
}

func (e *Viga) Error() string {
	s := fmt.Sprintf("[%s] %s: %s", e.Liik, e.Allikas, e.Teade)
	if e.URL != "" {
		s += " (" + e.URL + ")"
	}
	if e.Pohjus != nil {
		s += ": " + e.Pohjus.Error()
	}
	return s
}

func (e *Viga) Unwrap() error { return e.Pohjus }

// Uus loob vea.
func Uus(liik Liik, allikas, url, teade string, pohjus error) *Viga {
	return &Viga{Liik: liik, Allikas: allikas, URL: url, Teade: teade, Pohjus: pohjus}
}

// Struktuur on lühivorm struktuurimuutuse veale.
func Struktuur(allikas, url, teade string) *Viga {
	return Uus(StruktuurMuutunud, allikas, url, teade, nil)
}

// LiikOn kontrollib vea liiki.
func LiikOn(err error, l Liik) bool {
	var v *Viga
	return errors.As(err, &v) && v.Liik == l
}

// LiikVeast tagastab vea liigi või "" kui tegu pole Viga tüüpi veaga.
func LiikVeast(err error) Liik {
	var v *Viga
	if errors.As(err, &v) {
		return v.Liik
	}
	return ""
}
