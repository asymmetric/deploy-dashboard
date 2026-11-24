package comin

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	in := `# HELP comin_deployment_info Deployment info
# TYPE comin_deployment_info gauge
comin_deployment_info{status="done",out_path="/nix/store/abc-nixos",msg="a \"q\" b"} 1
comin_fetch_count 42 1700000000
`
	got, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d samples, want 2", len(got))
	}
	d := got[0]
	if d.Name != "comin_deployment_info" || d.Value != 1 {
		t.Errorf("sample 0 = %+v", d)
	}
	if d.Labels["out_path"] != "/nix/store/abc-nixos" || d.Labels["msg"] != `a "q" b` {
		t.Errorf("labels = %v", d.Labels)
	}
	if got[1].Name != "comin_fetch_count" || got[1].Value != 42 {
		t.Errorf("sample 1 = %+v", got[1])
	}
}

func TestParseMalformed(t *testing.T) {
	for _, in := range []string{`m{a="x} 1`, `m{a=x} 1`, `m`, `m{a="x"}`} {
		if _, err := Parse(strings.NewReader(in)); err == nil {
			t.Errorf("Parse(%q): want error", in)
		}
	}
}
