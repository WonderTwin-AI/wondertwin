package identity_test

import (
	"errors"
	"testing"

	"github.com/wondertwin-ai/wondertwin/twinkit/identity"
)

func TestTwinTwinID(t *testing.T) {
	tw := identity.Twin{
		Tier:      identity.TierCommercial,
		Publisher: identity.PublisherWonderTwin,
		Vendor:    "modern-treasury",
	}
	if got, want := tw.TwinID(), identity.TwinID("twin-modern-treasury"); got != want {
		t.Fatalf("TwinID() = %q, want %q", got, want)
	}
}

func TestTwinString(t *testing.T) {
	tests := []struct {
		name string
		tw   identity.Twin
		want string
	}{
		{
			name: "commercial wondertwin",
			tw:   identity.Twin{Tier: identity.TierCommercial, Publisher: identity.PublisherWonderTwin, Vendor: "stripe"},
			want: "commercial:wondertwin/stripe",
		},
		{
			name: "community third-party",
			tw:   identity.Twin{Tier: identity.TierCommunity, Publisher: "acme", Vendor: "stripe"},
			want: "community:acme/stripe",
		},
		{
			name: "multi-word vendor",
			tw:   identity.Twin{Tier: identity.TierCommercial, Publisher: identity.PublisherWonderTwin, Vendor: "modern-treasury"},
			want: "commercial:wondertwin/modern-treasury",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.tw.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseQualified(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    identity.Twin
		wantErr bool
	}{
		{
			name: "commercial wondertwin stripe",
			in:   "commercial:wondertwin/stripe",
			want: identity.Twin{Tier: identity.TierCommercial, Publisher: identity.PublisherWonderTwin, Vendor: "stripe"},
		},
		{
			name: "community third-party",
			in:   "community:acme/stripe",
			want: identity.Twin{Tier: identity.TierCommunity, Publisher: "acme", Vendor: "stripe"},
		},
		{
			name: "multi-word vendor",
			in:   "commercial:wondertwin/modern-treasury",
			want: identity.Twin{Tier: identity.TierCommercial, Publisher: identity.PublisherWonderTwin, Vendor: "modern-treasury"},
		},
		{
			name: "forward-compat tier (enterprise)",
			in:   "enterprise:wondertwin/stripe",
			want: identity.Twin{Tier: "enterprise", Publisher: identity.PublisherWonderTwin, Vendor: "stripe"},
		},
		{name: "bare name rejected", in: "stripe", wantErr: true},
		{name: "tier-only rejected", in: "commercial:stripe", wantErr: true},
		{name: "missing vendor rejected", in: "commercial:wondertwin/", wantErr: true},
		{name: "missing publisher rejected", in: "commercial:/stripe", wantErr: true},
		{name: "uppercase rejected", in: "Commercial:wondertwin/stripe", wantErr: true},
		{name: "leading digit publisher rejected", in: "commercial:1acme/stripe", wantErr: true},
		{name: "leading digit vendor rejected", in: "commercial:wondertwin/1stripe", wantErr: true},
		{name: "underscore rejected", in: "commercial:wondertwin/modern_treasury", wantErr: true},
		{name: "double slash rejected", in: "commercial:wondertwin//stripe", wantErr: true},
		{name: "trailing slash rejected", in: "commercial:wondertwin/stripe/", wantErr: true},
		{name: "empty rejected", in: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := identity.ParseQualified(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseQualified(%q) = %v, want error", tt.in, got)
				}
				if !errors.Is(err, identity.ErrInvalidQualified) {
					t.Errorf("error = %v, want wraps ErrInvalidQualified", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseQualified(%q) unexpected error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("ParseQualified(%q) = %+v, want %+v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseQualifiedRoundTrip(t *testing.T) {
	inputs := []string{
		"commercial:wondertwin/stripe",
		"community:acme/modern-treasury",
		"enterprise:wondertwin/twilio",
	}
	for _, in := range inputs {
		t.Run(in, func(t *testing.T) {
			tw, err := identity.ParseQualified(in)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if got := tw.String(); got != in {
				t.Errorf("round-trip: got %q, want %q", got, in)
			}
		})
	}
}

func TestTwinValidate(t *testing.T) {
	tests := []struct {
		name    string
		tw      identity.Twin
		wantErr bool
	}{
		{name: "valid", tw: identity.Twin{Tier: identity.TierCommercial, Publisher: identity.PublisherWonderTwin, Vendor: "stripe"}},
		{name: "valid forward-compat tier", tw: identity.Twin{Tier: "enterprise", Publisher: identity.PublisherWonderTwin, Vendor: "stripe"}},
		{name: "empty tier", tw: identity.Twin{Publisher: identity.PublisherWonderTwin, Vendor: "stripe"}, wantErr: true},
		{name: "uppercase tier", tw: identity.Twin{Tier: "Commercial", Publisher: identity.PublisherWonderTwin, Vendor: "stripe"}, wantErr: true},
		{name: "empty publisher", tw: identity.Twin{Tier: identity.TierCommercial, Vendor: "stripe"}, wantErr: true},
		{name: "empty vendor", tw: identity.Twin{Tier: identity.TierCommercial, Publisher: identity.PublisherWonderTwin}, wantErr: true},
		{name: "underscore vendor", tw: identity.Twin{Tier: identity.TierCommercial, Publisher: identity.PublisherWonderTwin, Vendor: "modern_treasury"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.tw.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseTwinID(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    identity.TwinID
		wantErr bool
	}{
		{name: "simple", in: "twin-stripe", want: "twin-stripe"},
		{name: "multi-word", in: "twin-modern-treasury", want: "twin-modern-treasury"},
		{name: "missing prefix", in: "stripe", wantErr: true},
		{name: "wrong prefix", in: "twn-stripe", wantErr: true},
		{name: "empty vendor", in: "twin-", wantErr: true},
		{name: "uppercase", in: "twin-Stripe", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := identity.ParseTwinID(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseTwinID(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseTwinID(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
