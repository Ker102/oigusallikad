# oigusallikad – täielik juhend

Eesti ja ELi õigusallikad otse algallikast, käsurealt ja MCP serverina:

- **Riigi Teataja:** seaduste ja määruste sätted ametlikust XMList, ka mineviku
  kuupäeva seisuga kehtinud redaktsioonist; akti sisukord; otsing akti sees.
- **Kohtulahendid:** Eesti kohtulahendite otsing (kõik astmed). Lahendi
  tekst tuleb Riigikohtu lahendite andmebaasist või Riigi Teatajast. Valida
  saab üksikuid punkte.
- **EUR-Lex:** ELi õigusaktide artiklid ning Euroopa Kohtu otsuste ja
  kohtujuristide ettepanekute punktid ja allmärkused eesti keeles, ELi
  Väljaannete Talituse Cellar liidesest.

Eesmärk on säästa aega ja agendi konteksti: agent küsib ainult vajaliku
sätte, punkti või artikli, mitte tervet dokumenti või veebilehte.

## Põhimõte: vaikseid vigu ei ole

Allikad muudavad aeg-ajalt oma ülesehitust. Tööriist on ehitatud nii, et
muutus annab **vea**, mitte tühja või vale tulemuse:

| Vea liik | Tähendus | Väljumiskood |
| --- | --- | --- |
| `ei_leitud` | allikas vastas korrektselt, et sellist dokumenti või sätet ei ole | 3 |
| `struktuur_muutunud` | vastus ei vasta oodatud kujule, tööriista tuleb uuendada | 4 |
| `blokeeritud` | allikas nõuab inimkontrolli (Cloudflare, AWS WAF) või keelas päringu | 5 |
| `kattesaamatu` | võrguviga, ajalõpp või serveri viga | 6 |
| `soltuvus_puudub` | kohalik abiprogramm (`pdftotext`) puudub | 7 |
| `vigane_sisend` | sisend on vigane (nt vale CELEX number) | 2 |

Näiteid kontrollidest:
- JSON vastuses peavad olema kindlad väljad. Kui otsing teatab „3 tulemust“,
  aga loend on tühi, on see struktuuriviga.
- XML juurelement peab olema `<oigusakt>` ja selles peab olema vähemalt üks
  paragrahv.
- Laaditud kohtulahendi tekstis peab esinema küsitud asja number.
- Kui küsitud punkte ei leita, öeldakse, millised puuduvad.
- Cloudflare'i ja WAF-i kontrolllehed tuvastatakse. Tööriist **ei ürita
  blokeeringutest mööda minna.**

MCP vastustes lisatakse struktuuri- ja blokeerimisvigadele selgitus: „ära
järelda, et dokumenti pole“.

### Kui tööriist on katki

Vigade `struktuur_muutunud`, `blokeeritud` ja ootamatute vigade korral
annab tööriist (ja MCP kaudu agent) kasutajale need sammud:

1. **Uuenda:** `go install github.com/Ker102/oigusallikad/cmd/oigusallikad@latest`.
2. **Vaata teadaolevaid probleeme:** https://github.com/Ker102/oigusallikad/issues
3. **Leia katkine allikas:** `oigusallikad kontroll -t`.
4. **Teata või paranda:** `oigusallikad veateade` käivitab kontrollid ja
   koostab eeltäidetud GitHubi probleemi lingi. GitHubi kasutajaga saab
   parandada ise ja saata pull requesti (vt „Kui allikas muutub“).

Seni kasutab agent sama dokumendi jaoks muud allikat ja ütleb, kust tekst
pärineb. Vea `ei_leitud` korral allikas vastas korrektselt, seega pole vaja
midagi parandada.

**Ära lisa probleemi kirjeldusse kliendi andmeid ega konfidentsiaalseid
dokumente.** Hoidla on avalik.

### Kanaarilinnud

`oigusallikad kontroll` käivitab päringud teadaolevate dokumentide vastu ja
kontrollib, et vastuses on oodatud tekst. Kontrollid:

- THS § 98 lg 3;
- THS § 88¹ lg 1 p 3 (ülaindeks);
- JäätS 2023. a redaktsioon (kuupäeva järgi valik);
- kohtulahendite otsing;
- ringkonnakohtu PDF-lahend;
- Riigikohtu lahend;
- Euroopa Kohtu otsus nii uues kui vanas vormingus;
- kohtujuristi ettepaneku allmärkus;
- direktiivi artikkel.

```
OK    RT säte: THS § 98 lg 3                                    1.758s
OK    Riigikohus (RIKOS): 3-15-443/54 p 13                      1.555s
OK    EUR-Lex kohtuotsus: C-626/22 p 94                          771ms
…
```

GitHubi töövoog `.github/workflows/allikate-kontroll.yml` teeb seda iga päev.
Kui allikas muutub, ebaõnnestub töövoog ja GitHub teavitab hoidla jälgijaid.

## Allikad ja liidesed

| Allikas | Liides | Märkus |
| --- | --- | --- |
| Riigi Teataja aktid | `riigiteataja.ee/public-api/api/v1/akt/…` (JSON, ametlik XML) | ilma brauserita |
| Riigi Teataja kohtulahendid | `…/kohtuteave/otsing/kohtulahendid` (JSON), lahendi PDF | PDF → tekst `pdftotext` abil |
| Riigikohus | `rikos.rik.ee` (Riigikohtu lahendite andmebaas, HTML) | riigikohus.ee kasutab sama andmebaasi |
| EUR-Lex | `publications.europa.eu/resource/celex/…` (Cellar REST) | eur-lex.europa.eu on WAF-i taga, seda ei kasutata |

**Teadaolevad piirangud:**

- **Riigi Teataja pealkirjaotsing** on Cloudflare'i kontrolli taga, seega
  tööriist seda ei kasuta. Seaduse leiab lühendi (`THS`, `JäätS`, `HMS`) või
  pealkirja osa järgi. Määruse puhul anna Riigi Teataja link või akti ID (nt
  `101072026014`).
- **curia.europa.eu** blokeerib automaatpäringud. Euroopa Kohtu tekstid on
  samad Cellaris.
- **Kohtulahendite otsing** tagastab esimese tulemuslehe. Kohtuastme ja
  kuupäeva filtrid rakendatakse sellele lehele.
- **Aktide lisad** (nt määruse lisa 2) on eraldi failid, enamasti PDF.
  `lisa` teisendab PDFi tekstiks, kuid tabelite numbrid võivad moonduda,
  seega lisatakse alati hoiatus. Tsiteeritavad väärtused kontrolli PDFist.
  Näide: määruse nr 21 lisa 2 ametlikus PDFis on kroomi piirväärtus „05“
  (tõenäoliselt 0,5).
- **Esialgsed tõlked** (värsked kohtujuristi ettepanekud) märgitakse
  hoiatusega, sest sõnastus ja numeratsioon võivad muutuda.

## Paigaldamine

Vaja on Go 1.25 või uuemat ning PDF-lahendite jaoks `pdftotext`:

```bash
# macOS
brew install poppler
# Debian/Ubuntu
sudo apt-get install poppler-utils
```

```bash
go install github.com/Ker102/oigusallikad/cmd/oigusallikad@latest
```

## Kasutamine käsurealt

Vaikimisi väljund on JSON koos päritoluga (URL, laadimisaeg, SHA-256). `-t`
annab inimloetava teksti.

```bash
# säte, ka mineviku seisuga
oigusallikad sate -t THS 98 -lg 3
oigusallikad sate -t THS "88^1" -lg 1 -p 3
oigusallikad sate -t JäätS 28 -lg 4 -kp 2023-06-01
oigusallikad sate -t 101072026014 "4^1"          # määrus ID järgi

# akti struktuur ja otsing akti sees
oigusallikad sisukord -t KeÜS
oigusallikad otsi-aktist -t JäätS eeltöötl
oigusallikad redaktsioonid -t THS -piir 5

# akti lisad (määruse lisa 2)
oigusallikad lisad -t 101072026014
oigusallikad lisa -t 101072026014 2 -kp 2023-06-01

# kohtulahendid
oigusallikad lahendid -t koldetuhk
oigusallikad lahendid -t -aste riigikohus "jäätmevaldaja vastutus"
oigusallikad lahend -t -ylevaade 3-22-1501/35     # punktide algused
oigusallikad lahend -t 3-15-443/54 -punktid 12,13

# EUR-Lex
oigusallikad eurlex -t 32008L0098 -artikkel 14,15
oigusallikad eurlex -t C-626/22 -punktid 94
oigusallikad eurlex -t C-730/23 -liik ettepanek -allmarkused 49
oigusallikad eurlex -t C-172/08 -punktid 35-39 -keel EN

# allikate tervis
oigusallikad kontroll -t
```

Kohtuasja number teisendatakse automaatselt CELEX numbriks: `C-626/22` →
`62022CJ0626`, ettepanek `62022CC0626`.

### Vahemälu

Vastused salvestatakse kasutaja vahemälukausta (Linuxis
`~/.cache/oigusallikad`). Aegumine:

| Mis | Aegub |
| --- | --- |
| aktide metaandmed | 6 h |
| aktide tekst | 24 h |
| kohtulahendite otsing | 1 h |
| kohtulahendid | 30 päeva |
| EUR-Lex | 7 päeva |

`-varske` laeb uuesti ja `-vahemalu -` lülitab vahemälu välja. Päringute
vahel on sama allika puhul vähemalt 0,5 s.

## Kasutamine MCP serverina

```bash
claude mcp add oigusallikad -- oigusallikad mcp
```

Claude Desktop (`claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "oigusallikad": { "command": "/tee/oigusallikad", "args": ["mcp"] }
  }
}
```

| Tööriist | Mida teeb |
| --- | --- |
| `rt_satte_tekst` | paragrahvi, lõike või punkti tekst; `kuupaev` valib redaktsiooni |
| `rt_akti_sisukord` | akti peatükid ja paragrahvid |
| `rt_otsi_aktist` | lõiked, kus esinevad kõik sõnad |
| `rt_redaktsioonid` | redaktsioonid ja kehtivusajad |
| `rt_akti_lisad` | akti lisade loetelu või ühe lisa tekst |
| `kohtulahendid_otsi` | lahendite otsing teksti või numbri järgi |
| `kohtulahend` | lahendi punktid, ülevaade või tekst |
| `eurlex_dokument` | ELi akti artiklid, kohtulahendi punktid, allmärkused |
| `allikate_kontroll` | kanaarilinnud (kas allikad töötavad) |

## Arendus

```bash
go test ./...                 # võrguühenduseta testid (kohalik testserver)
go run ./cmd/oigusallikad kontroll -t -varske   # päris allikate vastu
```

Võrguühenduseta testid simuleerivad:
- Cloudflare'i ja WAF-i blokeeringut;
- 404- ja 5xx-vastuseid;
- vale sisutüüpi;
- muutunud JSON-välju;
- tühja tulemuste loendit;
- paragrahvideta XML-i;
- redaktsiooni valikut kuupäeva järgi;
- mõlemat Cellari HTML-vormingut.

### Kui allikas muutub

1. `oigusallikad kontroll -t` näitab, milline allikas ja mis ebaõnnestus.
2. Vaata vastust brauseris ja võrdle koodiga vastavas paketis (vt
   „Ülesehitus“).
3. Lisa muutunud vormingu kohta test (`*_test.go`, kohalik testserver) ja
   paranda parser.
4. `go test ./...` ja `oigusallikad kontroll -t -varske` peavad läbima.
5. Saada parandus: tee hoidlast koopia (fork), haru, commit ja pull request
   aadressile https://github.com/Ker102/oigusallikad. Kirjelda, mis allikas
   muutus ja kuidas seda kontrollisid.

### Ülesehitus

```
cmd/oigusallikad/        käsurida, MCP käivitus
internal/klient/         HTTP: vahemälu, viisakas sagedus, kordused, blokeeringu tuvastus
internal/viga/           vealiigid
internal/rt/             Riigi Teataja aktid (XML, redaktsioonid)
internal/kohtulahendid/  kohtulahendite otsing ja tekst (RT, Riigikohus)
internal/eurlex/         Cellar: artiklid, punktid, allmärkused
internal/tekst/          HTML/PDF → tekst, lahendi punktideks jaotamine
internal/kontroll/       kanaarilinnud
skills/oigusallikad/     agendi oskus (skill): millal ja kuidas tööriista kasutada
internal/mcpserver/      MCP tööriistad
```

## Õiguslik märkus

Tööriist laeb avalikke õigusallikaid nende avalike liideste kaudu, viisaka
päringusagedusega. Iga vastus sisaldab algallika linki: enne tsiteerimist
kontrolli teksti algallikas.

## Litsents

Apache License 2.0 koos tingimusega **Commons Clause** (vt `LICENSE` ja
`NOTICE`). Lihtsalt öeldes:

- tööriista võib vabalt kasutada, ka ettevõttes ja kliendi töös;
- seda võib muuta ja edasi jagada, kui `LICENSE` ja `NOTICE` (autori
  märge) jäävad alles;
- tööriista ennast ega sellel põhinevat teenust ei tohi **müüa** (sh tasu
  eest majutada või pakkuda tasulist tuge, mille väärtus tuleneb peamiselt
  sellest tööriistast).

Tegu ei ole OSI mõttes avatud lähtekoodiga litsentsiga ("source-available").
Muudeks kasutusviisideks küsi luba autorilt.
