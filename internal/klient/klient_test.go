package klient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Ker102/oigusallikad/internal/viga"
)

func uusTestKlient(t *testing.T) *Klient {
	return Uus(Valikud{Vahemalu: t.TempDir(), Vahe: time.Millisecond, IlmaKordusteta: true})
}

func TestVealiigid(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/cloudflare", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("cf-mitigated", "challenge")
		w.WriteHeader(403)
		w.Write([]byte("<html><title>Just a moment...</title></html>"))
	})
	mux.HandleFunc("/cf200", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<!DOCTYPE html><html><head><title>Just a moment...</title></head></html>"))
	})
	mux.HandleFunc("/waf", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-amzn-waf-action", "challenge")
		w.WriteHeader(202)
	})
	mux.HandleFunc("/puudub", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })
	mux.HandleFunc("/server", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) })
	mux.HandleFunc("/html", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html>hooldustööd</html>"))
	})
	mux.HandleFunc("/tyhi", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	k := uusTestKlient(t)
	cases := []struct {
		tee  string
		sisu string
		liik viga.Liik
	}{
		{"/cloudflare", "", viga.Blokeeritud},
		{"/cf200", "", viga.Blokeeritud},
		{"/waf", "", viga.Blokeeritud},
		{"/puudub", "", viga.EiLeitud},
		{"/server", "", viga.Kattesaamatu},
		{"/html", "json", viga.StruktuurMuutunud},
		{"/tyhi", "json", viga.StruktuurMuutunud},
	}
	for _, c := range cases {
		_, err := k.Tee(context.Background(), Paring{Allikas: "test", URL: srv.URL + c.tee, OodatudSisu: c.sisu})
		if !viga.LiikOn(err, c.liik) {
			t.Errorf("%s: ootasin %s, sain %v", c.tee, c.liik, err)
		}
	}
}

func TestVahemalu(t *testing.T) {
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	k := uusTestKlient(t)
	p := Paring{Allikas: "test", URL: srv.URL + "/x", TTL: time.Hour, OodatudSisu: "json"}
	v1, err := k.Tee(context.Background(), p)
	if err != nil || v1.Vahemalust {
		t.Fatalf("esimene päring: %v %+v", err, v1)
	}
	v2, err := k.Tee(context.Background(), p)
	if err != nil || !v2.Vahemalust || n != 1 {
		t.Fatalf("teine päring peab tulema vahemälust: %v vahemälust=%v päringuid=%d", err, v2.Vahemalust, n)
	}
	if v1.SHA256 != v2.SHA256 {
		t.Fatal("räsi erineb")
	}
}
