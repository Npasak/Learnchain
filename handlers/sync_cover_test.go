package handlers

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestIsPrivateOrLocalHost(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{"localhost", true},
		{"127.0.0.1", true},
		{"10.87.194.92", true},
		{"10.87.143.48", true},
		{"172.16.5.10", true},
		{"192.168.1.100", true},
		{"169.254.1.1", true},
		{"script.google.com", false},
		{"8.8.8.8", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := isPrivateOrLocalHost(tc.host); got != tc.want {
			t.Errorf("isPrivateOrLocalHost(%q) = %v, want %v", tc.host, got, tc.want)
		}
	}
}

func TestFetchCoverFromPeers(t *testing.T) {
	wantBytes := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F'}
	const fname = "test_peer_cover_sync.jpg"

	t.Cleanup(func() {
		_ = os.Remove(filepath.Join(coversDir, fname))
	})
	_ = os.Remove(filepath.Join(coversDir, fname))

	htmlPeer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html>not an image</html>"))
	}))
	defer htmlPeer.Close()

	var sawNoPeerFetch bool
	imgPeer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/covers/"+fname {
			sawNoPeerFetch = (r.URL.Query().Get("no_peer_fetch") == "1")
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write(wantBytes)
			return
		}
		http.NotFound(w, r)
	}))
	defer imgPeer.Close()

	h := &SyncHandler{
		activeNodes: map[string]ActiveNodeInfo{
			"peer-html": {NodeID: "peer-html", LanURL: htmlPeer.URL},
			"peer-img":  {NodeID: "peer-img", LanURL: imgPeer.URL},
		},
	}

	got, ok := h.FetchCoverFromPeers(fname)
	if !ok {
		t.Fatalf("FetchCoverFromPeers(%q) returned ok=false, want true", fname)
	}
	if !sawNoPeerFetch {
		t.Errorf("expected ?no_peer_fetch=1 query parameter on peer cover request")
	}
	if string(got) != string(wantBytes) {
		t.Errorf("fetched bytes = %v, want %v", got, wantBytes)
	}
	cached, err := os.ReadFile(filepath.Join(coversDir, fname))
	if err != nil {
		t.Fatalf("expected cover to be cached in %s: %v", coversDir, err)
	}
	if string(cached) != string(wantBytes) {
		t.Errorf("cached bytes = %v, want %v", cached, wantBytes)
	}
}

func TestFetchCoverFromGAS(t *testing.T) {
	wantBytes := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	const fname = "test_gas_cover_sync.png"

	t.Cleanup(func() {
		_ = os.Remove(filepath.Join(coversDir, fname))
	})
	_ = os.Remove(filepath.Join(coversDir, fname))

	gasServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("action") == "get_cover" && r.URL.Query().Get("filename") == fname {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success":     true,
				"filename":    fname,
				"data_base64": base64.StdEncoding.EncodeToString(wantBytes),
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer gasServer.Close()

	h := &SyncHandler{
		cfg: SyncConfig{
			GasURL: gasServer.URL + "/script.google.com/macros/s/test/exec",
		},
	}

	got, ok := h.FetchCoverFromPeers(fname)
	if !ok {
		t.Fatalf("FetchCoverFromPeers(%q) via GAS fallback returned ok=false, want true", fname)
	}
	if string(got) != string(wantBytes) {
		t.Errorf("fetched bytes = %v, want %v", got, wantBytes)
	}
	cached, err := os.ReadFile(filepath.Join(coversDir, fname))
	if err != nil {
		t.Fatalf("expected cover to be cached in %s: %v", coversDir, err)
	}
	if string(cached) != string(wantBytes) {
		t.Errorf("cached bytes = %v, want %v", cached, wantBytes)
	}
}
