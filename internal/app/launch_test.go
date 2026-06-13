package app

import (
	"testing"

	"github.com/wlame/vibrator/internal/mount"
)

func TestMountVolumesAndDirs(t *testing.T) {
	rs := []mount.Resolved{
		{Path: "/data/refs", ReadOnly: true},
		{Path: "/work/lib", ReadOnly: false},
	}
	vols := mountVolumes(rs)
	if len(vols) != 2 {
		t.Fatalf("got %d volumes, want 2", len(vols))
	}
	if vols[0].Host != "/data/refs" || vols[0].Container != "/data/refs" || !vols[0].ReadOnly {
		t.Fatalf("vol0 = %+v", vols[0])
	}
	if vols[1].ReadOnly {
		t.Fatalf("vol1 should be writable: %+v", vols[1])
	}
	dirs := mountDirs(rs)
	if len(dirs) != 2 || dirs[0] != "/data/refs" || dirs[1] != "/work/lib" {
		t.Fatalf("dirs = %v", dirs)
	}
}
