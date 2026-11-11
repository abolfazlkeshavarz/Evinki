package legacy

import "testing"

func TestModelKey(t *testing.T) {
	cases := map[string]string{
		"جاروبرقی خانگی اوینکی FZ530":  "FZ530",
		"Evinki FZ-530 Performance":     "FZ530",
		"fz530":                         "FZ530",
		"جاروبرقی FZ۵۳۰":                "FZ530",
		"X100 Pro":                      "X100",
		"فیلتر هپا":                     "",
	}
	for in, want := range cases {
		if got := ModelKey(in); got != want {
			t.Errorf("ModelKey(%q)=%q want %q", in, got, want)
		}
	}
	if ModelKey("", "FZ530") != "FZ530" {
		t.Error("falls through to the next part")
	}
}
