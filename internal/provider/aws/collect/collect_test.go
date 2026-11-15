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
	if len(seen) != 21 {
		t.Fatal(len(seen))
	}
}
func TestShorts(t *testing.T) {
	want := []string{"acm", "aurora", "cloudfront", "dynamodb", "ebs", "ec2", "eip", "eks", "elasticache", "elb", "iam", "lambda", "nat", "rds", "route53", "s3", "sg", "snapshot", "sqs"}
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
func TestTask7GlobalRegistry(t *testing.T) {
	want := map[string]bool{"aws.cloudfront.distribution": true, "aws.iam.user": true, "aws.iam.group": true, "aws.iam.role": true, "aws.route53.hostedzone": true}
	for _, c := range All() {
		if want[c.Type] && !c.Global {
			t.Fatalf("%s should run globally", c.Type)
		}
	}
}
