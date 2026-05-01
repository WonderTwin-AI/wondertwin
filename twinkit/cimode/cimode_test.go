package cimode

import "testing"

func TestDetectFromEnv(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		env  []string
		want Detection
	}{
		{
			name: "empty env is local",
			env:  nil,
			want: Detection{Mode: ModeLocal},
		},
		{
			name: "github actions",
			env: []string{
				"CI=true",
				"GITHUB_ACTIONS=true",
				"GITHUB_RUN_ID=12345",
				"GITHUB_REPOSITORY_OWNER=acme",
			},
			want: Detection{
				Mode:     ModeCI,
				Platform: "github-actions",
				RunID:    "12345",
				OrgHint:  "acme",
			},
		},
		{
			name: "circleci",
			env: []string{
				"CIRCLECI=true",
				"CIRCLE_WORKFLOW_ID=wf-1",
				"CIRCLE_PROJECT_USERNAME=acme",
			},
			want: Detection{
				Mode:     ModeCI,
				Platform: "circleci",
				RunID:    "wf-1",
				OrgHint:  "acme",
			},
		},
		{
			name: "buildkite",
			env: []string{
				"BUILDKITE=true",
				"BUILDKITE_BUILD_ID=bk-9",
				"BUILDKITE_ORGANIZATION_SLUG=acme",
			},
			want: Detection{
				Mode:     ModeCI,
				Platform: "buildkite",
				RunID:    "bk-9",
				OrgHint:  "acme",
			},
		},
		{
			name: "gitlab",
			env: []string{
				"GITLAB_CI=true",
				"CI_PIPELINE_ID=42",
				"CI_PROJECT_NAMESPACE=acme/widgets",
			},
			want: Detection{
				Mode:     ModeCI,
				Platform: "gitlab",
				RunID:    "42",
				OrgHint:  "acme/widgets",
			},
		},
		{
			name: "generic CI",
			env:  []string{"CI=true"},
			want: Detection{Mode: ModeCI, Platform: "generic"},
		},
		{
			name: "github actions wins over generic",
			env:  []string{"CI=true", "GITHUB_ACTIONS=true"},
			want: Detection{Mode: ModeCI, Platform: "github-actions"},
		},
		{
			name: "CI=false is local",
			env:  []string{"CI=false"},
			want: Detection{Mode: ModeLocal},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := DetectFromEnv(tc.env)
			if got != tc.want {
				t.Fatalf("DetectFromEnv() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestModeString(t *testing.T) {
	if got := ModeLocal.String(); got != "local" {
		t.Errorf("ModeLocal.String() = %q, want %q", got, "local")
	}
	if got := ModeCI.String(); got != "ci" {
		t.Errorf("ModeCI.String() = %q, want %q", got, "ci")
	}
}
