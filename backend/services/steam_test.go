package services

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetCS2Playtime(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/IPlayerService/GetOwnedGames/v1/" || r.URL.Query().Get("include_played_free_games") != "1" {
			t.Errorf("unexpected request: %s", r.URL)
		}
		switch r.URL.Query().Get("steamid") {
		case "public":
			w.Write([]byte(`{"response":{"game_count":1,"games":[{"appid":730,"playtime_2weeks":42,"playtime_forever":208744}]}}`))
		case "no-recent":
			w.Write([]byte(`{"response":{"game_count":1,"games":[{"appid":730,"playtime_forever":600}]}}`))
		default: // private game details
			w.Write([]byte(`{"response":{}}`))
		}
	}))
	defer srv.Close()
	old := steamBaseURL
	steamBaseURL = srv.URL
	defer func() { steamBaseURL = old }()
	s := NewSteamService("key")

	total, recent, ok, err := s.GetCS2Playtime("public")
	if err != nil || !ok || int(total) != 3479 || recent != 0.7 {
		t.Errorf("public: got (%v, %v, %v, %v), want (3479.07, 0.7, true, nil)", total, recent, ok, err)
	}
	if total, recent, ok, _ := s.GetCS2Playtime("no-recent"); !ok || total != 10 || recent != 0 {
		t.Errorf("no recent play: got (%v, %v, %v)", total, recent, ok)
	}
	if _, _, ok, err := s.GetCS2Playtime("private"); ok || err != nil {
		t.Errorf("private: got ok=%v err=%v, want false, nil", ok, err)
	}
}

// Only the formats that need no network call are covered here; vanity URLs
// are resolved through the Steam API.
func TestParseSteamInput(t *testing.T) {
	s := NewSteamService("")

	cases := []struct {
		input   string
		wantID  string
		wantErr bool
	}{
		{"https://steamcommunity.com/profiles/76561198000000001", "76561198000000001", false},
		{"https://steamcommunity.com/profiles/76561198000000001/", "76561198000000001", false},
		{"  steamcommunity.com/profiles/76561198000000001  ", "76561198000000001", false},
		{"76561198000000001", "76561198000000001", false},
		{"abc", "", true},
		{"https://steamcommunity.com/profiles/zouse/", "", true}, // vanity name under /profiles/
		{"not-a-steam-id-but-long", "", true},
		{"7656119800000000", "", true}, // 16 digits
		{"", "", true},
	}
	for _, c := range cases {
		id, vanity, err := s.ParseSteamInput(c.input)
		if (err != nil) != c.wantErr {
			t.Errorf("ParseSteamInput(%q): err=%v, wantErr=%v", c.input, err, c.wantErr)
			continue
		}
		if id != c.wantID || vanity != "" {
			t.Errorf("ParseSteamInput(%q) = (%q, %q), want (%q, \"\")", c.input, id, vanity, c.wantID)
		}
	}
}
