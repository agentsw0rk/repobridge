package updatecheck

import "testing"

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want int
	}{
		{name: "equal with v prefix", a: "v0.10.4", b: "0.10.4", want: 0},
		{name: "newer patch", a: "v0.10.5", b: "v0.10.4", want: 1},
		{name: "older minor", a: "v0.9.9", b: "v0.10.0", want: -1},
		{name: "newer minor", a: "v0.11.0", b: "v0.10.9", want: 1},
		{name: "newer major", a: "v1.0.0", b: "v0.99.99", want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CompareVersions(tt.a, tt.b)
			if err != nil {
				t.Fatalf("CompareVersions() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("CompareVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestCompareVersionsRejectsMalformedVersions(t *testing.T) {
	for _, version := range []string{"", "dev", "main", "v1", "v1.2", "v1.2.x", "v1.2.3.4"} {
		t.Run(version, func(t *testing.T) {
			if _, err := CompareVersions(version, "v1.2.3"); err == nil {
				t.Fatalf("CompareVersions(%q, v1.2.3) error = nil, want error", version)
			}
		})
	}
}

func TestIsReleaseVersion(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{version: "v0.10.4", want: true},
		{version: "0.10.4", want: true},
		{version: "dev", want: false},
		{version: "", want: false},
	}
	for _, tt := range tests {
		if got := IsReleaseVersion(tt.version); got != tt.want {
			t.Fatalf("IsReleaseVersion(%q) = %v, want %v", tt.version, got, tt.want)
		}
	}
}
