package dockerguard

import (
	"os"
	"path/filepath"
	"testing"
)

func capPalworld(g *Guard) { g.cur().byName["palworld"].maxMemory = 16 << 30 }

func TestMaxMemoryBoundsCreates(t *testing.T) {
	h := newHarness(t, scopesJSON, capPalworld)
	host := func(b map[string]any) map[string]any { return b["HostConfig"].(map[string]any) }
	cases := []struct {
		name string
		set  map[string]any
		code int
	}{
		{"at the ceiling", map[string]any{"Memory": 16 << 30}, 201},
		{"under it", map[string]any{"Memory": 8 << 30, "MemorySwap": 16 << 30, "ShmSize": 1 << 30}, 201},
		{"no Memory limit at all", map[string]any{}, 403},
		{"Memory zero", map[string]any{"Memory": 0}, 403},
		{"over the ceiling", map[string]any{"Memory": 17 << 30}, 403},
		{"unlimited swap", map[string]any{"Memory": 1 << 30, "MemorySwap": -1}, 403},
		{"swap over twice the ceiling", map[string]any{"Memory": 1 << 30, "MemorySwap": 33 << 30}, 403},
		{"huge shm", map[string]any{"Memory": 1 << 30, "ShmSize": 64 << 30}, 403},
	}
	for _, c := range cases {
		body := composeBody(t)
		hc := host(body)
		delete(hc, "Memory")
		for k, v := range c.set {
			hc[k] = v
		}
		if r := h.request("POST", create, body); r.code != c.code {
			t.Errorf("%s: %s, want %d", c.name, r, c.code)
		}
	}
}

func TestNoCeilingLeavesMemoryAsItWas(t *testing.T) {
	h := newHarness(t, scopesJSON)
	body := composeBody(t)
	body["HostConfig"].(map[string]any)["Memory"] = 1 << 40
	if r := h.request("POST", create, body); r.code != 201 {
		t.Errorf("without max_memory: %s", r)
	}
}

func TestPolicyMaxMemoryReachesManagedScopesOnly(t *testing.T) {
	dir := t.TempDir()
	pol := filepath.Join(dir, "policy.yaml")
	managed := filepath.Join(dir, "scopes.json")
	if err := os.WriteFile(pol, []byte(testPolicy+"max_memory: 16g\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(managed, []byte("["+scopeJSON("valheim", "lloesche/valheim-server", "")+"]"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := Sources{Base: `[{"name":"panel-own","images":["crowsnest/x"]}]`, ManagedFile: managed, PolicyFile: pol}.Load()
	if err != nil || len(res.Scopes) != 2 {
		t.Fatalf("load: %+v %v", res, err)
	}
	for _, s := range res.Scopes {
		want := int64(0)
		if s.Name == "valheim" {
			want = 16 << 30
		}
		if s.maxMemory != want {
			t.Errorf("%s: maxMemory = %d, want %d", s.Name, s.maxMemory, want)
		}
	}
	if _, err := LoadScopes(`[{"name":"x","images":["a/b"],"max_memory":"1g"}]`); err == nil {
		t.Error("a scope key set its own memory ceiling")
	}
	if err := os.WriteFile(pol, []byte(testPolicy+"max_memory: lots\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPolicy(pol); err == nil {
		t.Error("a bad max_memory was accepted")
	}
}

func TestParseSize(t *testing.T) {
	for in, want := range map[string]int64{"512m": 512 << 20, "16g": 16 << 30, "16GiB": 16 << 30, "2048k": 2 << 20, "100": 100} {
		if got, err := ParseSize(in); err != nil || got != want {
			t.Errorf("ParseSize(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "0", "-1g", "g", "1.5g", "99999999999t"} {
		if _, err := ParseSize(bad); err == nil {
			t.Errorf("ParseSize(%q) accepted", bad)
		}
	}
}

func TestNamePatternCovered(t *testing.T) {
	list := []string{"gs-*", "minecraft"}
	for e, want := range map[string]bool{
		"gs-a*": true, "gs-*": true, "gs-one": true, "minecraft": true,
		"g*": false, "*": false, "gs*": false, "minecraft*": false, "other": false, "other*": false,
	} {
		if got := NamePatternCovered(list, e); got != want {
			t.Errorf("NamePatternCovered(%v, %q) = %v, want %v", list, e, got, want)
		}
	}
}
