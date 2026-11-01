// Package tekst teisendab HTMLi ja PDFi tekstiks ning jaotab kohtulahendid
// nummerdatud punktideks.
package tekst

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

var plokk = map[string]bool{
	"p": true, "div": true, "br": true, "tr": true, "li": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"table": true, "td": true, "th": true, "section": true, "article": true, "dt": true, "dd": true, "blockquote": true, "pre": true,
}

// HTMLTekstiks teisendab HTMLi ridadeks: plokkelemendid algavad uuelt realt,
// skriptid ja stiilid jäetakse välja.
func HTMLTekstiks(b []byte) (string, error) {
	doc, err := html.Parse(bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style" || n.Data == "head" || n.Data == "noscript") {
			return
		}
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
		}
		if n.Type == html.ElementNode && plokk[n.Data] {
			sb.WriteString("\n")
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if n.Type == html.ElementNode && plokk[n.Data] {
			sb.WriteString("\n")
		}
	}
	walk(doc)
	return PuhastaRead(sb.String()), nil
}

var (
	tyhikud = regexp.MustCompile(`[ \t\x{00A0}\x{2002}\x{2003}\x{2009}\x{200B}]+`)
	ridu    = regexp.MustCompile(`\n\s*\n+`)
)

// PuhastaRead normaliseerib tühikud ja tühjad read.
func PuhastaRead(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "­", "")
	s = tyhikud.ReplaceAllString(s, " ")
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	s = strings.Join(lines, "\n")
	s = ridu.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

// ErrPDFTeisendajaPuudub tagastatakse, kui pdftotext pole paigaldatud.
var ErrPDFTeisendajaPuudub = errors.New("pdftotext puudub (paigalda poppler-utils / brew install poppler)")

// PDFTekstiks teisendab PDFi tekstiks programmiga pdftotext (poppler).
func PDFTekstiks(ctx context.Context, b []byte) (string, error) {
	return pdfTekstiks(ctx, b, false)
}

// PDFTabelinaTekstiks säilitab veergude paigutuse (tabelid, nt aktide lisad).
func PDFTabelinaTekstiks(ctx context.Context, b []byte) (string, error) {
	return pdfTekstiks(ctx, b, true)
}

func pdfTekstiks(ctx context.Context, b []byte, layout bool) (string, error) {
	path, err := exec.LookPath("pdftotext")
	if err != nil {
		return "", ErrPDFTeisendajaPuudub
	}
	args := []string{"-enc", "UTF-8", "-nopgbrk"}
	if layout {
		args = append(args, "-layout")
	}
	args = append(args, "-", "-")
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Stdin = bytes.NewReader(b)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", errors.New("pdftotext: " + strings.TrimSpace(errb.String()) + ": " + err.Error())
	}
	return PuhastaRead(out.String()), nil
}

// Punkt on kohtulahendi nummerdatud punkt.
type Punkt struct {
	Nr    int    `json:"nr"`
	Tekst string `json:"tekst"`
}

var punktiAlgus = regexp.MustCompile(`^(\d{1,3})\.(?:\s+(.*))?$`)

// JaotaPunktideks jaotab kohtulahendi teksti punktideks. Punkt algab realt,
// mis koosneb numbrist ja punktist ("12.") või algab nii ("12. Tekst …").
// Arvestatakse ainult kasvavat järjestust, et välistada loetelud ja
// kuupäevad. Lehe jalused ("8(10)", "3-15-1620") eemaldatakse.
//
// Lahendis võib olla mitu "1."-ga algavat jada (resolutsiooni punktid,
// seejärel põhjenduste punktid). Iga "1." alustab uut kandidaatjada ja
// tagastatakse pikim jada.
func JaotaPunktideks(t string) []Punkt {
	lines := strings.Split(t, "\n")
	var parim, jada []Punkt
	var cur *Punkt
	var sb strings.Builder
	jargmine := 1
	flush := func() {
		if cur != nil {
			cur.Tekst = strings.TrimSpace(ridu.ReplaceAllString(sb.String(), "\n"))
			jada = append(jada, *cur)
			cur = nil
		}
	}
	lopetaJada := func() {
		flush()
		if len(jada) >= len(parim) {
			parim = jada
		}
		jada = nil
	}
	for _, ln := range lines {
		l := strings.TrimSpace(ln)
		if m := punktiAlgus.FindStringSubmatch(l); m != nil {
			n, _ := strconv.Atoi(m[1])
			uus := n == 1
			// Lubame väikese hüppe (nt puuduv punkt), aga mitte tagasi.
			if uus || (n >= jargmine && n <= jargmine+3) {
				if uus {
					lopetaJada()
				} else {
					flush()
				}
				cur = &Punkt{Nr: n}
				sb.Reset()
				if m[2] != "" {
					sb.WriteString(m[2])
					sb.WriteString(" ")
				}
				jargmine = n + 1
				continue
			}
		}
		if cur != nil && !onLeheJalus(l) {
			sb.WriteString(l)
			sb.WriteString("\n")
		}
	}
	lopetaJada()
	return parim
}

var jalus = regexp.MustCompile(`^(\d{1,3}\(\d{1,3}\)|\d-\d{1,2}-\d{1,6}(?:/\d+)?)$`)

func onLeheJalus(l string) bool { return jalus.MatchString(l) }

// ValiPunktid tagastab soovitud punktid ja puuduvate punktide loendi.
func ValiPunktid(koik []Punkt, soovitud []int) ([]Punkt, []int) {
	idx := map[int]Punkt{}
	for _, p := range koik {
		idx[p.Nr] = p
	}
	var out []Punkt
	var puudu []int
	for _, n := range soovitud {
		if p, ok := idx[n]; ok {
			out = append(out, p)
		} else {
			puudu = append(puudu, n)
		}
	}
	return out, puudu
}

var vahemik = regexp.MustCompile(`^(\d+)\s*[-–]\s*(\d+)$`)

// ParsiPunktid loeb "12,13,20-22" kujul loendi.
func ParsiPunktid(s string) ([]int, error) {
	var out []int
	seen := map[int]bool{}
	for _, osa := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || r == ' ' }) {
		if m := vahemik.FindStringSubmatch(osa); m != nil {
			a, _ := strconv.Atoi(m[1])
			b, _ := strconv.Atoi(m[2])
			if b < a || b-a > 200 {
				return nil, errors.New("vigane vahemik: " + osa)
			}
			for i := a; i <= b; i++ {
				if !seen[i] {
					seen[i] = true
					out = append(out, i)
				}
			}
			continue
		}
		n, err := strconv.Atoi(osa)
		if err != nil || n <= 0 {
			return nil, errors.New("vigane punkti number: " + osa)
		}
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	sort.Ints(out)
	return out, nil
}
