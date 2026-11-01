package mcpserver

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Ker102/oigusallikad/internal/eurlex"
	"github.com/Ker102/oigusallikad/internal/klient"
	"github.com/Ker102/oigusallikad/internal/kohtulahendid"
	"github.com/Ker102/oigusallikad/internal/rt"
)

func TestTooriistadJaVead(t *testing.T) {
	ctx := context.Background()
	k := klient.Uus(klient.Valikud{Vahemalu: "-", Vahe: time.Millisecond, IlmaKordusteta: true})
	srv := Uus("test", Allikad{RT: rt.Uus(k), Kohtud: kohtulahendid.Uus(k), EL: eurlex.Uus(k)})
	st, ct := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	nimed := map[string]bool{}
	for _, tl := range tools.Tools {
		nimed[tl.Name] = true
	}
	for _, n := range []string{"rt_satte_tekst", "rt_akti_sisukord", "rt_otsi_aktist", "rt_redaktsioonid", "kohtulahendid_otsi", "kohtulahend", "eurlex_dokument", "allikate_kontroll"} {
		if !nimed[n] {
			t.Errorf("tööriist puudub: %s", n)
		}
	}
	// vigane sisend jõuab agendini tööriista veana (mitte protokolli veana)
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "eurlex_dokument", Arguments: map[string]any{"viide": "pole-celex"}})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("ootasin tööriista viga")
	}
	txt := res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(txt, "vigane_sisend") {
		t.Errorf("vea liik peab olema tekstis: %s", txt)
	}
}
