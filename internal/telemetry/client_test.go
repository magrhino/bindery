package telemetry

import "testing"

func TestIsReleaseVersion(t *testing.T) {
	cases := []struct {
		version string
		want    bool
	}{
		// Release tags — both with and without the leading "v" since the
		// docker image build keeps "v1.7.0" while goreleaser strips it.
		{"v1.7.0", true},
		{"1.7.0", true},
		{"v0.0.1", true},
		{"v10.20.30", true},

		// Non-release labels CI emits today.
		{"dev", false},               // go run / go build with no -ldflags
		{"dev-abc1234", false},       // development branch interim image
		{"sha-abc1234", false},       // non-release branch interim image
		{"v1.7.0-3-gabc1234", false}, // git describe form (commits past a tag)
		{"v1.7.0-rc.1", false},       // pre-release (not currently used; safe to revisit)
		{"", false},                  // misconfigured / empty
		{"latest", false},            // image-tag style, not a real version
		{"v1.7", false},              // truncated
		{"1.7.0.1", false},           // four-component, not semver
		{"v1.7.0 ", false},           // whitespace-padded
		{"main", false},              // branch name accidentally injected
	}
	for _, tc := range cases {
		if got := isReleaseVersion(tc.version); got != tc.want {
			t.Errorf("isReleaseVersion(%q) = %v, want %v", tc.version, got, tc.want)
		}
	}
}
