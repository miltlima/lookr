package collect

import (
	"context"
	"testing"
)

func TestCollectVolumes(t *testing.T) {
	r, e := collectVolumes(context.Background(), &fakeEC2{}, Scope{"123", "us-east-1"})
	if e != nil || len(r) != 3 {
		t.Fatalf("%v %v", r, e)
	}
	if r[0].Provider != "aws" || r[0].Region != "us-east-1" || r[0].AccountID != "123" || r[0].ID == "" || r[0].ARN == "" {
		t.Fatal(r[0])
	}
}
func TestCollectSnapshots(t *testing.T) {
	r, e := collectSnapshots(context.Background(), &fakeEC2{}, Scope{"123", "us-east-1"})
	if e != nil || len(r) != 3 {
		t.Fatalf("%v %v", r, e)
	}
	if r[0].Provider != "aws" || r[0].Region != "us-east-1" || r[0].AccountID != "123" || r[0].ID == "" || r[0].ARN == "" {
		t.Fatal(r[0])
	}
	if r[0].Bool("source_volume_exists") || r[0].Bool("used_by_ami") {
		t.Fatal(r[0])
	}
}
func TestVolumeAttachmentAndSnapshotReferences(t *testing.T) {
	ctx := context.Background()
	r, e := collectVolumes(ctx, &fakeEC2{}, Scope{})
	if e != nil {
		t.Fatal(e)
	}
	if r[0].Str("attached_instance_id") != "" || r[1].Str("attached_instance_id") != "i-stopped" || r[1].Str("attached_instance_state") != "stopped" {
		t.Fatal(r)
	}
	r, e = collectSnapshots(ctx, &fakeEC2{}, Scope{})
	if e != nil {
		t.Fatal(e)
	}
	if !r[1].Bool("source_volume_exists") || !r[1].Bool("used_by_ami") {
		t.Fatal(r)
	}
}
