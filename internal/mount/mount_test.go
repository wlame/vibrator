package mount

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		wantPath string
		wantRO   bool
		wantErr  bool
	}{
		{"bare path is read-only", "/data/refs", "/data/refs", true, false},
		{"explicit ro", "/data/refs:ro", "/data/refs", true, false},
		{"explicit rw", "/work/lib:rw", "/work/lib", false, false},
		{"relative path kept as-is", "refs", "refs", true, false},
		{"trailing slash preserved", "/data/refs/:rw", "/data/refs/", false, false},
		{"unknown suffix errors", "/data/refs:xy", "", false, true},
		{"empty path errors", "", "", false, true},
		{"empty path with mode errors", ":rw", "", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Parse(%q): want error, got nil", tt.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%q): unexpected error: %v", tt.raw, err)
			}
			if got.HostPath != tt.wantPath || got.ReadOnly != tt.wantRO {
				t.Fatalf("Parse(%q) = {%q, ro=%v}, want {%q, ro=%v}",
					tt.raw, got.HostPath, got.ReadOnly, tt.wantPath, tt.wantRO)
			}
		})
	}
}
