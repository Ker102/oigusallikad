---
name: oigusallikad
description: Eesti ja ELi õigusallikate pärimine algallikast oigusallikad MCP tööriistadega – seaduste ja määruste sätted (ka mineviku kuupäeva redaktsioon) ja lisad Riigi Teatajast, Eesti kohtulahendid (Riigikohus, ringkonna-, maa- ja halduskohtud), ELi aktide artiklid ning Euroopa Kohtu otsuste ja kohtujuristide ettepanekute punktid ja allmärkused. Kasuta, kui on vaja tsiteerida või kontrollida Eesti/ELi õigusakti sätet, kohtulahendi punkti või CELEX/C-xx/yy dokumenti, kontrollida viidete täpsust (cite check) või leida, mis sõnastus kehtis teatud kuupäeval. Use for Estonian and EU legal sources, statute text, Riigi Teataja, Riigikohus, EUR-Lex, CJEU judgments, citation verification.
---

# Õigusallikad algallikast

Tööriistad: `rt_satte_tekst`, `rt_akti_sisukord`, `rt_otsi_aktist`,
`rt_redaktsioonid`, `rt_akti_lisad`, `kohtulahendid_otsi`, `kohtulahend`,
`eurlex_dokument`, `allikate_kontroll`. Käsurea vasted: `oigusallikad --help`.

## Põhireeglid

1. **Küsi kitsalt.** Ära lae tervet akti või lahendit, kui vaja on üht sätet
   või punkti. Nii säästad konteksti ja vähendad valesti tsiteerimise riski.
2. **Õige ajahetk.** Kui asjaolud on minevikus, kasuta `kuupaev` parameetrit
   (nt vaidlusaluse perioodi algus). Vastuses on redaktsiooni kehtivusaeg.
   Kui periood ulatub üle muudatuse, küsi mõlemad redaktsioonid.
3. **Tsiteeri päritoluga.** Iga vastuse `paritolu.url` on inimesele avatav
   link. Lisa see viite juurde. Ära koosta linke ise.
4. **Tsiteeri sõna-sõnalt** tööriista tagastatud tekstist. Ära parafraseeri
   jutumärkides.

## Töövood

**Säte:**
- tead paragrahvi: `rt_satte_tekst` (akt, paragrahv, loige, punkt, kuupaev);
- ei tea paragrahvi: `rt_akti_sisukord` või `rt_otsi_aktist` (sõna algus,
  nt „eeltöötl“), seejärel `rt_satte_tekst`;
- seadust leiab lühendi järgi (`THS`, `JäätS`, `HMS`, `KeÜS`). Määrusel
  lühendit pole: vaja on Riigi Teataja linki või akti ID-d. Kui seda pole,
  leia link veebiotsinguga ja anna see tööriistale.

**Akti lisa** (nt piirväärtuste tabel):
- `rt_akti_lisad` ilma numbrita loetleb lisad, numbriga tagastab teksti;
- PDF-tabelite numbrid võivad teisendamisel moonduda: ütle kasutajale, et
  tsiteeritav väärtus tuleb PDFist üle kontrollida, ja anna link.

**Eesti kohtulahend:**
- `kohtulahendid_otsi` (tekst või asja_nr, soovi korral aste ja kuupäevad);
- `kohtulahend` koos `ulevaade: true`, et leida õiged punktid;
- `kohtulahend` koos `punktid: "12,13"`.
- Lahendi number peab sisaldama dokumendi numbrit (nt `3-15-443/54`).

**EL:**
- `eurlex_dokument`: viide on CELEX (`32008L0098`) või kohtuasi (`C-626/22`);
- `artiklid` õigusaktide, `punktid` kohtulahendite jaoks;
- kohtujuristi ettepanek: `liik: "ettepanek"`. Sisu on sageli allmärkustes
  (`allmarkused: "49"`);
- hoiatus „ESIALGNE TÕLGE“ tuleb kasutajale edasi anda.

## Vead

| Vea liik | Mida teha |
| --- | --- |
| `ei_leitud` | Allikas vastas korrektselt. Kontrolli sisendit. Vajadusel kasuta muud allikat (veebiotsing, üldine veebikaabits) ja ütle kasutajale, kust tekst pärineb. |
| `vigane_sisend` | Paranda sisend veateate järgi. |
| `kattesaamatu` | Proovi hiljem uuesti. Seni kasuta muud allikat ja märgi see. |
| `struktuur_muutunud`, `blokeeritud`, muu ootamatu viga | Tööriist vajab tõenäoliselt parandust. **Ära järelda, et dokumenti pole.** Ütle kasutajale selgelt, et tööriist on katki, ja anna veas olevad sammud: uuendamine (`go install …@latest`), teadaolevad probleemid GitHubis, `oigusallikad kontroll -t`, `oigusallikad veateade` (eeltäidetud probleem) ja võimalus saata parandus (pull request), kui kasutajal on GitHub. Ütle ka, et probleemi juurde ei tohi lisada kliendi andmeid. Seni kasuta muud allikat ja märgi see. |

Kui vigu tuleb mitmest tööriistast, käivita `allikate_kontroll` ja teata
kasutajale, millised allikad on katki.

## Mida tööriist ei tee

- Riigi Teataja pealkirjaotsing on Cloudflare'i taga, seega seda ei kasutata.
  Kasuta lühendit, linki või ID-d.
- curia.europa.eu blokeerib automaatpäringud. Kasuta `eurlex_dokument`, sest
  tekstid on samad.
- Kommenteeritud väljaanded, Juridica ja muud tasulised allikad ei ole
  kättesaadavad.
- Tööriist ei hinda, kas kohtupraktika on endiselt kehtiv. Hilisemate
  lahendite kontroll on eraldi samm.
