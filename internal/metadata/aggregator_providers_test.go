package metadata

import "testing"

// PrimaryProviderName backs the add-flow provider-mismatch stamp (#2237): the
// API layer compares it against the provider a foreign ID routes to, so it
// must reflect the effective wired primary, normalized, and stay safe on the
// degenerate aggregator shapes handlers can hold (nil aggregator, no primary).
func TestPrimaryProviderName(t *testing.T) {
	cases := []struct {
		name string
		agg  *Aggregator
		want string
	}{
		{name: "nil aggregator", agg: nil, want: ""},
		{name: "no primary wired", agg: NewAggregator(nil), want: ""},
		{name: "hardcover primary", agg: NewAggregator(&mockProvider{name: "hardcover"}), want: "hardcover"},
		{name: "dnb primary", agg: NewAggregator(&mockProvider{name: "dnb"}), want: "dnb"},
		{name: "short alias is normalized", agg: NewAggregator(&mockProvider{name: "HC"}), want: "hardcover"},
		{name: "openlibrary default", agg: NewAggregator(&mockProvider{name: "openlibrary"}), want: "openlibrary"},
		{name: "unknown name passes through lowercased", agg: NewAggregator(&mockProvider{name: " Bogus "}), want: "bogus"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.agg.PrimaryProviderName(); got != tc.want {
				t.Fatalf("PrimaryProviderName() = %q, want %q", got, tc.want)
			}
		})
	}
}
