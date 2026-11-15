package checks

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"lookr/internal/model"
)

var testNow = time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)

func TestConfigurationChecks(t *testing.T) {
	tests := []struct {
		name, typ, check string
		bad, good        map[string]any
		severity         model.Severity
	}{
		{"admin ssh", "ec2.securitygroup", "sg.open_admin_port", map[string]any{"world_open_ports": []string{"22"}}, map[string]any{"world_open_ports": []string{"443"}}, model.SeverityHigh},
		{"admin rdp", "ec2.securitygroup", "sg.open_admin_port", map[string]any{"world_open_ports": []string{"3389"}}, nil, model.SeverityHigh},
		{"admin all", "ec2.securitygroup", "sg.open_admin_port", map[string]any{"world_open_ports": []string{"all"}}, nil, model.SeverityHigh},
		{"public database", "rds.instance", "rds.public", map[string]any{"publicly_accessible": true}, map[string]any{"publicly_accessible": false}, model.SeverityHigh},
		{"bucket block", "s3.bucket", "s3.public_access_not_blocked", map[string]any{"public_access_blocked": false}, map[string]any{"public_access_blocked": true}, model.SeverityHigh},
		{"console mfa", "iam.user", "iam.user_no_mfa", map[string]any{"has_console_password": true, "mfa_enabled": false}, map[string]any{"has_console_password": true, "mfa_enabled": true}, model.SeverityHigh},
		{"volume encryption", "ebs.volume", "ebs.unencrypted", map[string]any{"encrypted": false}, map[string]any{"encrypted": true}, model.SeverityMedium},
		{"database encryption", "rds.instance", "rds.unencrypted", map[string]any{"storage_encrypted": false}, map[string]any{"storage_encrypted": true}, model.SeverityMedium},
		{"database backup", "rds.instance", "rds.no_backups", map[string]any{"backup_retention_days": 0}, map[string]any{"backup_retention_days": 1}, model.SeverityMedium},
		{"imds", "ec2.instance", "ec2.imdsv1", map[string]any{"imds_v2_required": false, "imds_endpoint_enabled": true}, map[string]any{"imds_v2_required": true, "imds_endpoint_enabled": true}, model.SeverityMedium},
		{"access key", "iam.user", "iam.access_key_old", map[string]any{"oldest_active_key_age_days": 91}, map[string]any{"oldest_active_key_age_days": 90}, model.SeverityMedium},
		{"http", "cloudfront.distribution", "cloudfront.http_allowed", map[string]any{"viewer_protocol_policy": "allow-all"}, map[string]any{"viewer_protocol_policy": "redirect-to-https"}, model.SeverityMedium},
		{"tls", "cloudfront.distribution", "cloudfront.old_tls", map[string]any{"min_tls_version": "TLSv1.1_2016"}, map[string]any{"min_tls_version": "TLSv1.2_2021"}, model.SeverityMedium},
		{"eks", "eks.cluster", "eks.public_endpoint", map[string]any{"endpoint_public": true, "public_access_cidrs": []string{"0.0.0.0/0"}}, map[string]any{"endpoint_public": true, "public_access_cidrs": []string{"10.0.0.0/8"}}, model.SeverityMedium},
		{"runtime", "lambda.function", "lambda.deprecated_runtime", map[string]any{"runtime": "python3.10"}, map[string]any{"runtime": "python3.11"}, model.SeverityMedium},
		{"gp2", "ebs.volume", "ebs.gp2", map[string]any{"volume_type": "gp2"}, map[string]any{"volume_type": "gp3"}, model.SeverityLow},
		{"queue encryption", "sqs.queue", "sqs.unencrypted", map[string]any{"encrypted": false}, map[string]any{"encrypted": true}, model.SeverityLow},
		{"unused certificate", "acm.certificate", "acm.unused", map[string]any{"in_use_count": 0}, map[string]any{"in_use_count": 1}, model.SeverityLow},
		{"expiring certificate", "acm.certificate", "acm.expiring", map[string]any{"in_use_count": 1, "not_after": testNow.Add(29 * 24 * time.Hour).Format(time.RFC3339)}, map[string]any{"in_use_count": 1, "not_after": testNow.Add(31 * 24 * time.Hour).Format(time.RFC3339)}, model.SeverityHigh},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, good := range []bool{false, true} {
				config := tt.bad
				if good {
					config = tt.good
				}
				found := false
				for _, f := range Run(model.Resource{Type: "aws." + tt.typ, Config: config}, testNow) {
					if f.Check != tt.check {
						continue
					}
					found = true
					if f.Severity != tt.severity || f.Message == "" || f.Recommendation == "" {
						t.Errorf("bad finding: %+v", f)
					}
				}
				if found == good {
					t.Errorf("good=%v found=%v for %s", good, found, tt.check)
				}
			}
		})
	}
}

func TestCheckBoundaries(t *testing.T) {
	tests := []struct {
		typ, check string
		config     map[string]any
		want       bool
	}{
		{"iam.user", "iam.user_no_mfa", map[string]any{"has_console_password": false}, false},
		{"iam.user", "iam.access_key_old", map[string]any{"oldest_active_key_age_days": -1}, false},
		{"rds.instance", "rds.no_backups", map[string]any{"backup_retention_days": 0, "cluster_id": "aurora"}, false},
		{"eks.cluster", "eks.public_endpoint", map[string]any{"endpoint_public": false, "public_access_cidrs": []string{"0.0.0.0/0"}}, false},
		{"acm.certificate", "acm.expiring", map[string]any{"in_use_count": 1, "not_after": ""}, false},
		{"acm.certificate", "acm.expiring", map[string]any{"in_use_count": 0, "not_after": testNow.Add(24 * time.Hour).Format(time.RFC3339)}, false},
	}
	for _, tls := range []string{"SSLv3", "TLSv1", "TLSv1_2016", "TLSv1.1_2016", "", "TLSv1.2_2021"} {
		tests = append(tests, struct {
			typ, check string
			config     map[string]any
			want       bool
		}{"cloudfront.distribution", "cloudfront.old_tls", map[string]any{"min_tls_version": tls}, tls != "" && tls != "TLSv1.2_2021"})
	}
	for i, tt := range tests {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			found := false
			for _, f := range Run(model.Resource{Type: "aws." + tt.typ, Config: tt.config}, testNow) {
				if f.Check == tt.check {
					found = true
				}
			}
			if found != tt.want {
				t.Errorf("%s %+v: got %v", tt.check, tt.config, found)
			}
		})
	}
	r := model.Resource{Type: "aws.acm.certificate", Config: map[string]any{"in_use_count": 1, "not_after": testNow.Add(-time.Hour).Format(time.RFC3339)}}
	fs := Run(r, testNow)
	if len(fs) != 1 || fs[0].Message != "certificate has expired" {
		t.Fatalf("%+v", fs)
	}
}

func TestWasteChecks(t *testing.T) {
	for _, verdict := range []model.Verdict{model.VerdictIdle, model.VerdictOrphan, model.VerdictInUse, model.VerdictUnknown, model.VerdictNotEvaluated} {
		for _, amount := range []*float64{nil, ptr(49.99), ptr(50), ptr(80)} {
			r := model.Resource{Usage: model.Usage{Verdict: verdict, Reason: "test reason"}, Cost: model.Cost{MonthlyUSD: amount}}
			fs := Run(r, testNow)
			if verdict != model.VerdictIdle && verdict != model.VerdictOrphan {
				if len(fs) != 0 {
					t.Fatal(fs)
				}
				continue
			}
			wantSeverity := model.SeverityLow
			if amount != nil && *amount >= 50 {
				wantSeverity = model.SeverityMedium
			}
			if len(fs) != 1 || fs[0].Check != "usage."+string(verdict) || fs[0].Severity != wantSeverity || !strings.Contains(fs[0].Message, "test reason") {
				t.Fatalf("%+v", fs)
			}
			if amount != nil && !strings.Contains(fs[0].Message, fmt.Sprintf("$%.2f/mo", *amount)) {
				t.Fatal(fs)
			}
			if fs[0].Recommendation != "Review whether this resource is still needed" {
				t.Fatal(fs)
			}
		}
	}
}

func TestRunOrder(t *testing.T) {
	r := model.Resource{Type: "aws.ebs.volume", Config: map[string]any{"encrypted": false, "volume_type": "gp2"}, Usage: model.Usage{Verdict: model.VerdictOrphan}, Cost: model.Cost{MonthlyUSD: ptr(80)}}
	var got []string
	for _, f := range Run(r, testNow) {
		got = append(got, f.Check)
	}
	if !reflect.DeepEqual(got, []string{"ebs.unencrypted", "usage.orphan", "ebs.gp2"}) {
		t.Fatal(got)
	}
}

func TestRunNeverNil(t *testing.T) {
	if fs := Run(model.Resource{}, testNow); fs == nil || len(fs) != 0 {
		t.Fatal(fs)
	}
}

func TestUnknownMFADoesNotImplyDisabled(t *testing.T) {
	r := model.Resource{Type: "aws.iam.user", Config: map[string]any{"has_console_password": true}}
	for _, f := range Run(r, testNow) {
		if f.Check == "iam.user_no_mfa" {
			t.Fatal("missing MFA information must not imply disabled MFA")
		}
	}
}

func TestWasteRecommendationPerType(t *testing.T) {
	for _, typ := range []string{"ec2.instance", "ebs.volume", "ebs.snapshot", "ec2.eip", "ec2.natgateway", "elb.loadbalancer", "rds.instance", "rds.cluster", "elasticache.cluster", "eks.cluster", "lambda.function", "dynamodb.table", "sqs.queue", "cloudfront.distribution", "s3.bucket"} {
		r := model.Resource{Type: "aws." + typ, Usage: model.Usage{Verdict: model.VerdictIdle}}
		var recommendation string
		for _, f := range Run(r, testNow) {
			if f.Check == "usage.idle" {
				recommendation = f.Recommendation
			}
		}
		if recommendation == "" || recommendation == "Review whether this resource is still needed" {
			t.Errorf("missing advice for %s", typ)
		}
	}
}

func ptr(v float64) *float64 { return &v }
