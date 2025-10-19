package serialize

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/twinmind/newo-tool/internal/platform"
)

func TestGenerateAttributesYAML(t *testing.T) {
	testCases := []struct {
		name  string
		attrs []platform.CustomerAttribute
	}{
		{
			name:  "with attributes",
			attrs: []platform.CustomerAttribute{{IDN: "attr1"}},
		},
		{
			name:  "with nil slice",
			attrs: nil,
		},
		{
			name:  "with empty slice",
			attrs: []platform.CustomerAttribute{},
		},
	}

	want := "attributes: []\n"

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotBytes, err := GenerateAttributesYAML(tc.attrs)
			if err != nil {
				t.Fatalf("GenerateAttributesYAML() unexpected error: %v", err)
			}

			got := string(gotBytes)
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("GenerateAttributesYAML() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
