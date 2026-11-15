package collect

import "testing"

func TestAllUniqueTypes(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range All() {
		if seen[c.Type] || c.Short == "" {
			t.Fatal(c)
		}
		seen[c.Type] = true
	}
	if len(seen) != 13 {
		t.Fatal(len(seen))
	}
}
func TestShorts(t *testing.T) {
	want := []string{"aurora", "dynamodb", "ebs", "ec2", "eip", "elasticache", "elb", "nat", "rds", "s3", "sg", "snapshot", "sqs"}
	got := Shorts()
	if len(got) != len(want) {
		t.Fatal(got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatal(got)
		}
	}
}
