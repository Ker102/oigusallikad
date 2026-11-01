package kohtulahendid

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Ker102/oigusallikad/internal/klient"
	"github.com/Ker102/oigusallikad/internal/viga"
)

func serveri(t *testing.T, vastus string) *Klient {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("otsing peab olema POST, sain %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(vastus))
	}))
	t.Cleanup(srv.Close)
	baas = srv.URL
	return Uus(klient.Uus(klient.Valikud{Vahemalu: "-", Vahe: time.Millisecond, IlmaKordusteta: true}))
}

func TestOtsingStruktuur(t *testing.T) {
	cases := []struct {
		nimi, vastus string
		liik         viga.Liik
	}{
		{"kokku puudub", `{"tulemused":[]}`, viga.StruktuurMuutunud},
		{"kokku>0 aga tühi loend", `{"kokku":3,"tulemused":[]}`, viga.StruktuurMuutunud},
		{"väljad nimetati ümber", `{"kokku":1,"tulemused":[{"caseNumber":"3-1-1","id":5}]}`, viga.StruktuurMuutunud},
		{"pole JSON", `<html>`, viga.StruktuurMuutunud},
	}
	for _, c := range cases {
		k := serveri(t, c.vastus)
		_, err := k.Otsi(context.Background(), Valikud{Tekst: "koldetuhk"})
		if !viga.LiikOn(err, c.liik) {
			t.Errorf("%s: ootasin %s, sain %v", c.nimi, c.liik, err)
		}
	}
}

func TestOtsingFiltrid(t *testing.T) {
	k := serveri(t, `{"kokku":3,"tulemused":[
 {"kohtuasjaNumber":"3-15-443/54","toiminguEsitajaAsutus":"Riigikohtu halduskolleegium","lahendiKuulutamiseAeg":"2018-04-20T12:00:00+03:00","objektId":1,"avalikustatudFailiId":2},
 {"kohtuasjaNumber":"3-22-1501/35","toiminguEsitajaAsutus":"Tallinna Ringkonnakohtu halduskolleegium","lahendiKuulutamiseAeg":"2025-03-25T15:00:00+02:00","objektId":3,"avalikustatudFailiId":4},
 {"kohtuasjaNumber":"3-22-1501/13","toiminguEsitajaAsutus":"Tallinna Halduskohus","lahendiKuulutamiseAeg":"2023-07-10T15:00:00+03:00","objektId":5,"avalikustatudFailiId":6}]}`)
	o, err := k.Otsi(context.Background(), Valikud{Tekst: "x", Aste: "riigikohus"})
	if err != nil || len(o.Tulemused) != 1 || o.Tulemused[0].AsjaNr != "3-15-443/54" {
		t.Fatalf("aste: %v %+v", err, o)
	}
	o, _ = k.Otsi(context.Background(), Valikud{Tekst: "x", Alates: "2024-01-01"})
	if len(o.Tulemused) != 1 || o.Tulemused[0].Kuupaev != "2025-03-25" {
		t.Fatalf("kuupäev: %+v", o.Tulemused)
	}
	if _, err := k.Tekst(context.Background(), TekstiValikud{AsjaNr: "kolm"}); !viga.LiikOn(err, viga.Sisend) {
		t.Errorf("vigane asja number: %v", err)
	}
}
