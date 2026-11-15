package model

import (
	"reflect"
	"testing"
)

func TestAccessors(t *testing.T) {
	r := Resource{
		Type: "aws.ebs.volume", Region: "us-east-1", ID: "vol-1",
		Config: map[string]any{
			"i": 3, "i32": int32(4), "i64": int64(5), "f": 6.0, "s": "x",
			"b": true, "ss": []string{"a", "b"}, "fl": 1.5,
		},
	}
	if got := r.Key(); got != "aws.ebs.volume|us-east-1|vol-1" {
		t.Errorf("Key = %q", got)
	}
	for k, want := range map[string]int{"i": 3, "i32": 4, "i64": 5, "f": 6, "missing": 0, "s": 0} {
		if got := r.Int(k); got != want {
			t.Errorf("Int(%q) = %d, want %d", k, got, want)
		}
	}
	if r.Str("s") != "x" || r.Str("missing") != "" || r.Str("i") != "" {
		t.Error("Str mismatch")
	}
	if !r.Bool("b") || r.Bool("missing") || r.Bool("s") {
		t.Error("Bool mismatch")
	}
	if !reflect.DeepEqual(r.Strs("ss"), []string{"a", "b"}) || r.Strs("missing") != nil || r.Strs("s") != nil {
		t.Error("Strs mismatch")
	}
	if r.Float("fl") != 1.5 || r.Float("i") != 3 || r.Float("missing") != 0 || r.Float("s") != 0 {
		t.Error("Float mismatch")
	}
	var nilCfg Resource
	if nilCfg.Int("x") != 0 || nilCfg.Str("x") != "" {
		t.Error("nil config")
	}
}
