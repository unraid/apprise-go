package notify

import "testing"

func TestLautherPriorityParsing(t *testing.T) {
	tests := []struct {
		raw     string
		want    int
		wantErr bool
	}{
		{raw: "lowest", want: -2},
		{raw: "low", want: -1},
		{raw: "normal", want: 0},
		{raw: "high", want: 1},
		{raw: "emergency", want: 2},
		{raw: "2", want: 2},
		{raw: "not-a-priority", want: 0},
		{raw: "3", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.raw, func(t *testing.T) {
			got, err := parseLautherPriority(test.raw)
			if test.wantErr {
				if err == nil {
					t.Fatalf("parseLautherPriority(%q) succeeded, want error", test.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseLautherPriority(%q): %v", test.raw, err)
			}
			if got != test.want {
				t.Errorf("parseLautherPriority(%q) = %d, want %d", test.raw, got, test.want)
			}
		})
	}
}

func TestSignalgridBuildRequestRequiresChannel(t *testing.T) {
	parsed, err := ParseURL("signalgrid://CLIENTKEY")
	if err != nil {
		t.Fatalf("ParseURL: %v", err)
	}
	target, err := NewSignalgridTarget(parsed)
	if err != nil {
		t.Fatalf("NewSignalgridTarget: %v", err)
	}
	if _, err := target.BuildRequest("body", "title", NotifyInfo); err == nil {
		t.Fatal("BuildRequest succeeded without a Signalgrid channel")
	}
}
