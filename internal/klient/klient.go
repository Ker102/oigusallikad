// Package klient on ühine HTTP klient: viisakas päringusagedus, kordused
// ajutiste vigade korral, kettavahemälu ning blokeeringu ja HTML-vealehtede
// tuvastus.
package klient

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Ker102/oigusallikad/internal/viga"
)

// Versioon lisatakse User-Agent päisesse.
var Versioon = "0.1.0"

// Paring on üks HTTP päring.
type Paring struct {
	Allikas string // nt "riigiteataja", logide ja vigade jaoks
	Meetod  string
	URL     string
	Paised  map[string]string
	Keha    []byte
	// TTL: kui kaua vastust vahemälust kasutada (0 = ära kasuta vahemälu).
	TTL time.Duration
	// OodatudSisu: kui määratud, peab Content-Type sisaldama seda (nt "json").
	OodatudSisu string
	// MaxSuurus: vastuse maksimaalne suurus baitides (vaikimisi 50 MB).
	MaxSuurus int64
}

// Vastus on HTTP vastus koos päritoluga.
type Vastus struct {
	URL        string    `json:"url"`
	Staatus    int       `json:"-"`
	SisuTyyp   string    `json:"-"`
	Keha       []byte    `json:"-"`
	Laaditud   time.Time `json:"laaditud"`
	SHA256     string    `json:"sha256"`
	Vahemalust bool      `json:"vahemalust"`
}

// Klient on HTTP klient.
type Klient struct {
	http     *http.Client
	vahemalu string
	varske   bool
	mu       sync.Mutex
	viimane  map[string]time.Time
	vahe     time.Duration
	kordusi  int
}

// Valikud seadistavad klienti.
type Valikud struct {
	// Vahemalu kaust; tühi = kasutaja vahemälukaust/oigusallikad, "-" = välja lülitatud.
	Vahemalu string
	// Varske: ignoreeri vahemälu (kirjuta siiski uus vastus).
	Varske bool
	// Vahe: minimaalne aeg kahe päringu vahel sama hostiga (vaikimisi 500 ms).
	Vahe    time.Duration
	Ajalopp time.Duration
	// IlmaKordusteta: ära korda ajutiste vigade korral (testid).
	IlmaKordusteta bool
}

// Uus loob kliendi.
func Uus(v Valikud) *Klient {
	if v.Vahe == 0 {
		v.Vahe = 500 * time.Millisecond
	}
	if v.Ajalopp == 0 {
		v.Ajalopp = 60 * time.Second
	}
	dir := v.Vahemalu
	if dir == "" {
		if d, err := os.UserCacheDir(); err == nil {
			dir = filepath.Join(d, "oigusallikad")
		}
	}
	if dir == "-" {
		dir = ""
	}
	k := &Klient{vahemalu: dir, varske: v.Varske, viimane: map[string]time.Time{}, vahe: v.Vahe, kordusi: 2}
	if v.IlmaKordusteta {
		k.kordusi = 0
	}
	k.http = &http.Client{
		Timeout: v.Ajalopp,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("liiga palju ümbersuunamisi")
			}
			// EL Väljaannete Talituse Cellar suunab http:// aadressile; hoia https.
			if req.URL.Scheme == "http" {
				req.URL.Scheme = "https"
			}
			// Säilita Accept päised ümbersuunamisel (Go teeb seda vaikimisi
			// sama hosti puhul; tagame selle ka hostivahetusel).
			for _, h := range []string{"Accept", "Accept-Language"} {
				if v := via[0].Header.Get(h); v != "" {
					req.Header.Set(h, v)
				}
			}
			return nil
		},
	}
	return k
}

func (k *Klient) oota(ctx context.Context, host string) error {
	k.mu.Lock()
	jargmine := k.viimane[host].Add(k.vahe)
	nyyd := time.Now()
	if jargmine.Before(nyyd) {
		jargmine = nyyd
	}
	k.viimane[host] = jargmine
	k.mu.Unlock()
	d := time.Until(jargmine)
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

type vahemaluKirje struct {
	URL      string    `json:"url"`
	SisuTyyp string    `json:"sisu_tyyp"`
	Laaditud time.Time `json:"laaditud"`
}

func (k *Klient) votiFail(p Paring) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\n%s\n", p.Meetod, p.URL)
	for _, kk := range []string{"Accept", "Accept-Language", "Content-Type"} {
		fmt.Fprintf(h, "%s=%s\n", kk, p.Paised[kk])
	}
	h.Write(p.Keha)
	return filepath.Join(k.vahemalu, hex.EncodeToString(h.Sum(nil)))
}

// Tee teeb päringu. Tagastab vea (viga.Viga), kui vastus on blokeeritud,
// 404, serveri viga või ootamatu sisutüübiga.
func (k *Klient) Tee(ctx context.Context, p Paring) (*Vastus, error) {
	if p.Meetod == "" {
		p.Meetod = http.MethodGet
	}
	if p.MaxSuurus == 0 {
		p.MaxSuurus = 50 << 20
	}
	if p.TTL > 0 && k.vahemalu != "" && !k.varske {
		if v := k.loeVahemalust(p); v != nil {
			return v, nil
		}
	}
	u, err := url.Parse(p.URL)
	if err != nil {
		return nil, viga.Uus(viga.Sisend, p.Allikas, p.URL, "vigane URL", err)
	}
	var viimaneViga error
	for katse := 0; katse <= k.kordusi; katse++ {
		if katse > 0 {
			select {
			case <-ctx.Done():
				return nil, viga.Uus(viga.Kattesaamatu, p.Allikas, p.URL, "katkestatud", ctx.Err())
			case <-time.After(time.Duration(katse*katse) * time.Second):
			}
		}
		if err := k.oota(ctx, u.Host); err != nil {
			return nil, viga.Uus(viga.Kattesaamatu, p.Allikas, p.URL, "katkestatud", err)
		}
		v, err, korda := k.teeKord(ctx, p)
		if err == nil {
			if p.TTL > 0 && k.vahemalu != "" {
				k.kirjutaVahemallu(p, v)
			}
			return v, nil
		}
		viimaneViga = err
		if !korda {
			break
		}
	}
	return nil, viimaneViga
}

func (k *Klient) teeKord(ctx context.Context, p Paring) (*Vastus, error, bool) {
	req, err := http.NewRequestWithContext(ctx, p.Meetod, p.URL, bytes.NewReader(p.Keha))
	if err != nil {
		return nil, viga.Uus(viga.Sisend, p.Allikas, p.URL, "päringut ei saa koostada", err), false
	}
	req.Header.Set("User-Agent", "oigusallikad/"+Versioon+" (oigusallikate MCP; https://github.com/Ker102/oigusallikad)")
	for kk, vv := range p.Paised {
		req.Header.Set(kk, vv)
	}
	resp, err := k.http.Do(req)
	if err != nil {
		return nil, viga.Uus(viga.Kattesaamatu, p.Allikas, p.URL, "võrguviga", err), true
	}
	defer resp.Body.Close()
	keha, err := io.ReadAll(io.LimitReader(resp.Body, p.MaxSuurus+1))
	if err != nil {
		return nil, viga.Uus(viga.Kattesaamatu, p.Allikas, p.URL, "vastuse lugemine katkes", err), true
	}
	if int64(len(keha)) > p.MaxSuurus {
		return nil, viga.Uus(viga.StruktuurMuutunud, p.Allikas, p.URL, fmt.Sprintf("vastus on suurem kui %d baiti", p.MaxSuurus), nil), false
	}
	lopp := resp.Request.URL.String()
	ct := resp.Header.Get("Content-Type")

	if b := blokeeritud(resp, keha); b != "" {
		return nil, viga.Uus(viga.Blokeeritud, p.Allikas, lopp, b+". Tööriist ei ürita blokeeringust mööda minna; ava allikas brauseris või kasuta muud teed.", nil), false
	}
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		return nil, viga.Uus(viga.EiLeitud, p.Allikas, lopp, fmt.Sprintf("HTTP %d", resp.StatusCode), nil), false
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, viga.Uus(viga.Kattesaamatu, p.Allikas, lopp, "allikas piiras päringute arvu (HTTP 429)", nil), true
	case resp.StatusCode >= 500:
		return nil, viga.Uus(viga.Kattesaamatu, p.Allikas, lopp, fmt.Sprintf("serveri viga HTTP %d", resp.StatusCode), nil), true
	case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized:
		return nil, viga.Uus(viga.Blokeeritud, p.Allikas, lopp, fmt.Sprintf("juurdepääs keelatud (HTTP %d)", resp.StatusCode), nil), false
	case resp.StatusCode != http.StatusOK:
		return nil, viga.Uus(viga.StruktuurMuutunud, p.Allikas, lopp, fmt.Sprintf("ootamatu HTTP olek %d", resp.StatusCode), nil), false
	}
	if p.OodatudSisu != "" && !strings.Contains(strings.ToLower(ct), p.OodatudSisu) {
		return nil, viga.Struktuur(p.Allikas, lopp, fmt.Sprintf("ootasin sisutüüpi %q, sain %q", p.OodatudSisu, ct)), false
	}
	if len(bytes.TrimSpace(keha)) == 0 {
		return nil, viga.Struktuur(p.Allikas, lopp, "tühi vastus"), false
	}
	sum := sha256.Sum256(keha)
	return &Vastus{URL: lopp, Staatus: resp.StatusCode, SisuTyyp: ct, Keha: keha, Laaditud: time.Now().UTC(), SHA256: hex.EncodeToString(sum[:])}, nil, false
}

// blokeeritud tuvastab Cloudflare'i ja AWS WAF-i kontrolllehed.
func blokeeritud(resp *http.Response, keha []byte) string {
	if resp.Header.Get("cf-mitigated") != "" {
		return "allikas nõuab Cloudflare'i inimkontrolli (cf-mitigated: " + resp.Header.Get("cf-mitigated") + ")"
	}
	if resp.StatusCode == http.StatusAccepted && resp.Header.Get("x-amzn-waf-action") != "" {
		return "allikas nõuab AWS WAF kontrolli"
	}
	if resp.StatusCode == http.StatusAccepted && len(keha) < 20000 && bytes.Contains(bytes.ToLower(keha), []byte("challenge")) {
		return "allikas tagastas kontrolllehe (HTTP 202)"
	}
	start := keha
	if len(start) > 3000 {
		start = start[:3000]
	}
	low := bytes.ToLower(start)
	if bytes.Contains(low, []byte("<title>just a moment")) || bytes.Contains(low, []byte("cf-chl")) {
		return "allikas tagastas Cloudflare'i kontrolllehe"
	}
	return ""
}

func (k *Klient) loeVahemalust(p Paring) *Vastus {
	f := k.votiFail(p)
	meta, err := os.ReadFile(f + ".json")
	if err != nil {
		return nil
	}
	var m vahemaluKirje
	if json.Unmarshal(meta, &m) != nil || time.Since(m.Laaditud) > p.TTL {
		return nil
	}
	keha, err := os.ReadFile(f)
	if err != nil {
		return nil
	}
	sum := sha256.Sum256(keha)
	return &Vastus{URL: m.URL, Staatus: 200, SisuTyyp: m.SisuTyyp, Keha: keha, Laaditud: m.Laaditud, SHA256: hex.EncodeToString(sum[:]), Vahemalust: true}
}

func (k *Klient) kirjutaVahemallu(p Paring, v *Vastus) {
	if err := os.MkdirAll(k.vahemalu, 0o700); err != nil {
		return
	}
	f := k.votiFail(p)
	tmp := f + ".tmp"
	if os.WriteFile(tmp, v.Keha, 0o600) != nil {
		return
	}
	if os.Rename(tmp, f) != nil {
		return
	}
	b, _ := json.Marshal(vahemaluKirje{URL: v.URL, SisuTyyp: v.SisuTyyp, Laaditud: v.Laaditud})
	_ = os.WriteFile(f+".json", b, 0o600)
}
