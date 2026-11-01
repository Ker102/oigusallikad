package rt

import (
	"bytes"
	"encoding/xml"
	"io"
	"regexp"
	"strings"
)

// solm on minimaalne XML puu.
type solm struct {
	nimi   string
	attr   map[string]string
	lapsed []*solm
	tekst  string // ainult tekstisõlmedel (nimi == "")
}

func parsiXML(b []byte) (*solm, error) {
	dec := xml.NewDecoder(bytes.NewReader(b))
	dec.Strict = false
	juur := &solm{nimi: "#dokument"}
	pinu := []*solm{juur}
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			s := &solm{nimi: t.Name.Local, attr: map[string]string{}}
			for _, a := range t.Attr {
				s.attr[a.Name.Local] = a.Value
			}
			ylem := pinu[len(pinu)-1]
			ylem.lapsed = append(ylem.lapsed, s)
			pinu = append(pinu, s)
		case xml.EndElement:
			if len(pinu) > 1 {
				pinu = pinu[:len(pinu)-1]
			}
		case xml.CharData:
			ylem := pinu[len(pinu)-1]
			ylem.lapsed = append(ylem.lapsed, &solm{tekst: string(t)})
		}
	}
	return juur, nil
}

func (s *solm) laps(nimi string) *solm {
	for _, c := range s.lapsed {
		if c.nimi == nimi {
			return c
		}
	}
	return nil
}

func (s *solm) koik(nimi string) []*solm {
	var out []*solm
	for _, c := range s.lapsed {
		if c.nimi == nimi {
			out = append(out, c)
		}
	}
	return out
}

// otsi leiab kõik antud nimega järeltulijad (sügavuti).
func (s *solm) otsi(nimi string) []*solm {
	var out []*solm
	var walk func(n *solm)
	walk = func(n *solm) {
		for _, c := range n.lapsed {
			if c.nimi == nimi {
				out = append(out, c)
			}
			walk(c)
		}
	}
	walk(s)
	return out
}

var ylaindeks = map[rune]rune{'0': '⁰', '1': '¹', '2': '²', '3': '³', '4': '⁴', '5': '⁵', '6': '⁶', '7': '⁷', '8': '⁸', '9': '⁹'}

func ylaindeksiks(s string) string {
	var b strings.Builder
	for _, r := range s {
		if u, ok := ylaindeks[r]; ok {
			b.WriteRune(u)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// tekstSisu kogub sõlme teksti; <sup> muudetakse ülaindeksiks, muutmismärked
// jäetakse välja (need kogutakse eraldi).
func (s *solm) tekstSisu() string {
	var b strings.Builder
	var walk func(n *solm)
	walk = func(n *solm) {
		for _, c := range n.lapsed {
			switch c.nimi {
			case "":
				b.WriteString(c.tekst)
			case "muutmismarge", "kuvatavNr", "loigeNr", "alampunktNr", "paragrahvNr":
				// välja
			case "sup":
				b.WriteString(ylaindeksiks(c.tekstSisu()))
			default:
				walk(c)
			}
		}
	}
	walk(s)
	return strings.Join(strings.Fields(b.String()), " ")
}

var supRe = regexp.MustCompile(`<sup>\s*([0-9]+)\s*</sup>`)
var siltRe = regexp.MustCompile(`<[^>]+>`)

// kuvatavNr tagastab sõlme kuvatava numbri ("§ 88¹.", "(1)", "2)").
// RT XML-is on see CDATA sees HTML-ina (nt "§ 88<sup>1</sup>.").
func (s *solm) kuvatavNr() string {
	k := s.laps("kuvatavNr")
	if k == nil {
		return ""
	}
	raw := k.tekstSisu()
	raw = supRe.ReplaceAllStringFunc(raw, func(m string) string {
		return ylaindeksiks(supRe.FindStringSubmatch(m)[1])
	})
	return strings.TrimSpace(siltRe.ReplaceAllString(raw, ""))
}

// nr tagastab sõlme numbri koos ülaindeksiga, nt "88¹".
func nrUlaindeksiga(s *solm, silt string) string {
	n := s.laps(silt)
	if n == nil {
		return ""
	}
	t := strings.TrimSpace(n.tekstSisu())
	if y := n.attr["ylaIndeks"]; y != "" {
		t += ylaindeksiks(y)
	}
	return t
}

func (s *solm) muutmismargid() []string {
	var out []string
	for _, m := range s.koik("muutmismarge") {
		var osad []string
		if a := m.laps("avaldamismarge"); a != nil {
			rt := strings.TrimSpace(textOf(a, "RTosa"))
			kp := strings.TrimSpace(textOf(a, "avaldamineKuupaev"))
			art := strings.TrimSpace(textOf(a, "RTartikkel"))
			if rt != "" {
				osad = append(osad, rt+", "+eestiKuupaev(kp)+", "+art)
			}
		}
		if j := strings.TrimSpace(textOf(m, "joustumine")); j != "" {
			osad = append(osad, "jõust. "+eestiKuupaev(j))
		}
		if len(osad) > 0 {
			out = append(out, "["+strings.Join(osad, " - ")+"]")
		}
	}
	return out
}

func textOf(s *solm, laps string) string {
	if c := s.laps(laps); c != nil {
		return c.tekstSisu()
	}
	return ""
}

// eestiKuupaev: "2019-12-21" -> "21.12.2019".
func eestiKuupaev(iso string) string {
	if len(iso) >= 10 && iso[4] == '-' && iso[7] == '-' {
		return iso[8:10] + "." + iso[5:7] + "." + iso[0:4]
	}
	return iso
}
