// Package checks reports configuration risks without changing AWS resources.
package checks

import (
	"fmt"
	"slices"
	"sort"
	"time"

	"lookr/internal/model"
)

const (
	CertExpiryDays      = 30
	AccessKeyMaxAgeDays = 90
	WasteMediumUSD      = 50.0
)

// DeprecatedLambdaRuntimes was checked on 2026-11-15 against
// https://docs.aws.amazon.com/lambda/latest/dg/lambda-runtimes.html.
// Unknown identifiers are not presumed deprecated. Recheck this list on updates.
var DeprecatedLambdaRuntimes = map[string]bool{
	"nodejs": true, "nodejs4.3": true, "nodejs4.3-edge": true,
	"nodejs6.10": true, "nodejs8.10": true, "nodejs10.x": true,
	"nodejs12.x": true, "nodejs14.x": true, "nodejs16.x": true,
	"nodejs18.x": true, "nodejs20.x": true,
	"python2.7": true, "python3.6": true, "python3.7": true,
	"python3.8": true, "python3.9": true, "python3.10": true,
	"dotnetcore1.0": true, "dotnetcore2.0": true, "dotnetcore2.1": true,
	"dotnetcore3.1": true, "dotnet5.0": true, "dotnet6": true,
	"dotnet7": true, "dotnet8": true, "dotnet9": true,
	"java8": true, "go1.x": true, "ruby2.5": true,
	"ruby2.7": true, "ruby3.2": true, "provided": true, "provided.al2": true,
}

// Run returns findings sorted by severity and then check identifier.
func Run(r model.Resource, now time.Time) []model.Finding {
	findings := []model.Finding{}
	add := func(check string, severity model.Severity, message, advice string) {
		findings = append(findings, model.Finding{Check: check, Severity: severity, Message: message, Recommendation: advice})
	}
	// A missing configuration value is unknown, rather than evidence that a
	// security control is disabled (for example after a bucket AccessDenied).
	falseValue := func(key string) bool { v, ok := r.Config[key].(bool); return ok && !v }
	switch r.Type {
	case "aws.ec2.securitygroup":
		ports := r.Strs("world_open_ports")
		if slices.Contains(ports, "22") || slices.Contains(ports, "3389") || slices.Contains(ports, "all") {
			add("sg.open_admin_port", model.SeverityHigh, "administrative ports are open to the internet", "Restrict administrative ports to trusted addresses")
		}
	case "aws.rds.instance":
		if r.Bool("publicly_accessible") {
			add("rds.public", model.SeverityHigh, "database is publicly accessible", "Disable public accessibility or restrict the security group to known addresses")
		}
		if falseValue("storage_encrypted") {
			add("rds.unencrypted", model.SeverityMedium, "database storage is not encrypted", "Plan a migration to encrypted storage using a tested backup and restore procedure")
		}
		if _, ok := r.Config["backup_retention_days"]; ok && r.Int("backup_retention_days") == 0 && r.Str("cluster_id") == "" {
			add("rds.no_backups", model.SeverityMedium, "automated backups are disabled", "Enable automated backups and verify the recovery procedure")
		}
	case "aws.s3.bucket":
		if falseValue("public_access_blocked") {
			add("s3.public_access_not_blocked", model.SeverityHigh, "bucket does not have all public access blocks enabled", "Enable all public access blocks after checking required public access")
		}
	case "aws.iam.user":
		if r.Bool("has_console_password") && falseValue("mfa_enabled") {
			add("iam.user_no_mfa", model.SeverityHigh, "console user does not have MFA enabled", "Enable MFA for this console user")
		}
		if r.Int("oldest_active_key_age_days") > AccessKeyMaxAgeDays {
			add("iam.access_key_old", model.SeverityMedium, "active access key is more than 90 days old", "Rotate the key and verify its consumers before deactivating the old key")
		}
	case "aws.ebs.volume":
		if falseValue("encrypted") {
			add("ebs.unencrypted", model.SeverityMedium, "volume is not encrypted", "Create an encrypted copy and plan a tested migration")
		}
		if r.Str("volume_type") == "gp2" {
			add("ebs.gp2", model.SeverityLow, "volume uses gp2 storage", "Evaluate gp3 with equivalent IOPS and throughput before migrating")
		}
	case "aws.ec2.instance":
		if falseValue("imds_v2_required") {
			add("ec2.imdsv1", model.SeverityMedium, "instance accepts IMDSv1", "Require IMDSv2 after verifying application compatibility")
		}
	case "aws.cloudfront.distribution":
		if r.Str("viewer_protocol_policy") == "allow-all" {
			add("cloudfront.http_allowed", model.SeverityMedium, "distribution accepts unencrypted HTTP", "Redirect HTTP requests to HTTPS or require HTTPS")
		}
		if slices.Contains([]string{"SSLv3", "TLSv1", "TLSv1_2016", "TLSv1.1_2016"}, r.Str("min_tls_version")) {
			add("cloudfront.old_tls", model.SeverityMedium, "distribution permits a TLS version older than 1.2", "Use a minimum TLS policy of 1.2 or later after checking client compatibility")
		}
	case "aws.eks.cluster":
		if r.Bool("endpoint_public") && (slices.Contains(r.Strs("public_access_cidrs"), "0.0.0.0/0") || slices.Contains(r.Strs("public_access_cidrs"), "::/0")) {
			add("eks.public_endpoint", model.SeverityMedium, "cluster API endpoint is open to the internet", "Restrict public endpoint CIDRs or use private endpoint access")
		}
	case "aws.lambda.function":
		if DeprecatedLambdaRuntimes[r.Str("runtime")] {
			add("lambda.deprecated_runtime", model.SeverityMedium, "function uses a deprecated runtime", "Test and deploy the function on a supported runtime")
		}
	case "aws.sqs.queue":
		if falseValue("encrypted") {
			add("sqs.unencrypted", model.SeverityLow, "queue is not encrypted", "Enable server-side encryption after verifying producer and consumer permissions")
		}
	case "aws.acm.certificate":
		if _, ok := r.Config["in_use_count"]; ok && r.Int("in_use_count") == 0 {
			add("acm.unused", model.SeverityLow, "certificate is not associated with a resource", "Confirm the certificate is not needed before removing it")
		} else if r.Int("in_use_count") > 0 {
			expires, err := time.Parse(time.RFC3339, r.Str("not_after"))
			if err == nil && expires.Sub(now) < CertExpiryDays*24*time.Hour {
				message := "certificate expires in less than 30 days"
				if !expires.After(now) {
					message = "certificate has expired"
				}
				add("acm.expiring", model.SeverityHigh, message, "Renew or replace the certificate and verify deployment to associated resources")
			}
		}
	}
	if r.Usage.Verdict == model.VerdictIdle || r.Usage.Verdict == model.VerdictOrphan {
		severity := model.SeverityLow
		message := r.Usage.Reason
		if r.Cost.MonthlyUSD != nil {
			message += fmt.Sprintf(" ($%.2f/mo)", *r.Cost.MonthlyUSD)
			if *r.Cost.MonthlyUSD >= WasteMediumUSD {
				severity = model.SeverityMedium
			}
		}
		add("usage."+string(r.Usage.Verdict), severity, message, wasteAdvice(r))
	}
	rank := map[model.Severity]int{model.SeverityHigh: 0, model.SeverityMedium: 1, model.SeverityLow: 2}
	sort.Slice(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if rank[a.Severity] != rank[b.Severity] {
			return rank[a.Severity] < rank[b.Severity]
		}
		return a.Check < b.Check
	})
	return findings
}

func wasteAdvice(r model.Resource) string {
	switch r.Type {
	case "aws.ec2.instance":
		return "Review workload schedules and dependencies before stopping or terminating the instance"
	case "aws.ebs.volume":
		if r.Usage.Verdict == model.VerdictOrphan {
			return "Create a snapshot if the data may be needed, then delete the volume"
		}
		return "Review the attached instance and preserve needed data before removing the volume"
	case "aws.ebs.snapshot":
		return "Check recovery and retention requirements before deleting the snapshot"
	case "aws.ec2.eip":
		return "Check DNS and external dependencies before releasing the address"
	case "aws.ec2.natgateway":
		return "Verify routing and egress dependencies before removing the NAT gateway"
	case "aws.elb.loadbalancer":
		return "Check listeners, targets and DNS dependencies before removing the load balancer"
	case "aws.rds.instance":
		return "Verify application and recovery requirements and take a final snapshot before removing the database"
	case "aws.rds.cluster":
		return "Review cluster members and backup retention before removing the database cluster"
	case "aws.elasticache.cluster":
		return "Verify cache consumers and persistence requirements before resizing or removing the cache"
	case "aws.eks.cluster":
		return "Check scheduled workloads and self-managed nodes before removing the cluster"
	case "aws.lambda.function":
		return "Check triggers and infrequent schedules before removing the function"
	case "aws.dynamodb.table":
		return "Verify applications and backup requirements before reducing capacity or removing the table"
	case "aws.sqs.queue":
		return "Check producers, consumers and queued messages before removing the queue"
	case "aws.cloudfront.distribution":
		return "Check DNS, origins and scheduled traffic before removing the distribution"
	case "aws.s3.bucket":
		return "Check application dependencies and retention requirements before removing the empty bucket"
	default:
		return "Review whether this resource is still needed"
	}
}
