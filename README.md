# lookr

`lookr scan` inventories AWS resources, evaluates recent usage, projects monthly cost, and reports configuration findings. It only reads AWS APIs. Recommendations are text; no changes are applied.

## Install

Requires Go 1.25 or newer:

```sh
go build -o lookr .
# Or install from this checkout into your Go bin directory:
go install .
```

Use the standard AWS credential chain: environment variables, shared profiles, SSO, or an instance/task role. Sign in with the AWS CLI first for SSO profiles.

## Usage

```sh
lookr scan
lookr scan --profile production --regions us-east-1,sa-east-1 --type ec2,ebs,rds
lookr scan --days 14 --cost estimate --output json --out-file inventory.json
lookr scan --cost none --output csv > inventory.csv
lookr scan --cost none --no-metrics --output json
```

Former per-service commands such as `lookr ec2` are replaced by `lookr scan --type ec2`.

| Flag | Default | Meaning |
| --- | --- | --- |
| `--days` | `30` | Usage observation window; positive integer |
| `--regions` | all enabled | Comma-separated AWS regions |
| `--type` | all | Comma-separated short names below |
| `--output` | `table` | `table`, `json`, or `csv` |
| `--out-file` | stdout | Write the report to a file |
| `--cost` | `auto` | `auto`, `estimate`, or `none` |
| `--profile` | credential chain | AWS shared configuration profile |
| `--concurrency` | `8` | Maximum regions collected concurrently |
| `--no-metrics` | `false` | Disable CloudWatch usage queries and their charges |

Progress and warnings go to stderr; stdout contains only the report. Failed services/regions produce warnings while the scan continues, retaining partial resources where available. Argument errors, credential failures, and cancellation return a nonzero status. Completed scans with warnings return zero. Press Ctrl-C to cancel.

## Supported resources

| Short | Resources |
| --- | --- |
| `ec2` | EC2 instances |
| `ebs` | EBS volumes |
| `snapshot` | Account-owned EBS snapshots |
| `eip` | Elastic IP addresses |
| `nat` | NAT gateways |
| `sg` | Security groups |
| `elb` | Application, network, gateway, and classic load balancers |
| `rds` | RDS instances, including Aurora members |
| `aurora` | RDS/Aurora clusters |
| `elasticache` | Provisioned ElastiCache clusters |
| `eks` | EKS clusters |
| `lambda` | Lambda functions |
| `dynamodb` | DynamoDB tables |
| `sqs` | SQS queues |
| `cloudfront` | CloudFront distributions (global) |
| `s3` | S3 buckets (global inventory; regional storage metrics) |
| `iam` | IAM users, groups, and roles (global) |
| `acm` | ACM certificates |
| `route53` | Route53 hosted zones (global) |

Global inventory runs once regardless of selected regions. These 19 short names cover 21 resource types.

## Output and verdicts

The terminal table shows totals, idle/orphan cost, verdicts, cost sources, issue counts, and recommendations. Resources are sorted by verdict and decreasing cost.

JSON schema version `"1"` includes `generated_at`, `provider`, `account_id`, `window_days`, `regions`, `summary`, `resources`, and `warnings`. Resources contain identity, tags, creation time when available, service-specific `config`, `usage` (verdict, reason, window and metrics), `cost`, and `findings`. Missing cost is `null`, never an invented zero. Unknown security configuration fields are omitted rather than represented as disabled controls. Empty collections are arrays/objects. Summary includes monthly and waste costs, verdict counts, and finding counts by severity.

CSV has one row per resource:

```text
provider,account_id,region,type,id,arn,name,created_at,verdict,reason,monthly_usd,cost_source,cost_note,tags,config,metrics,findings
```

The last four columns contain JSON. Summary and warnings are omitted from CSV; warnings still appear on stderr.

| Verdict | Meaning |
| --- | --- |
| `in_use` | Observed activity or an explicit active association |
| `idle` | No activity in the window, or explicitly stopped/disabled |
| `orphan` | Unattached or missing an expected dependency |
| `unknown` | Missing evidence, failed metrics, or too new to evaluate |
| `not_evaluated` | Inventory only; no usage rule |

Resources with usage rules and younger than seven days are `unknown` when creation time is available. Successful empty CloudWatch counters count as zero; failed/incomplete queries do not. Missing gauges such as CPU and connections remain unknown. ElastiCache sums metrics across known nodes; missing node data prevents idle. Multi-Attach EBS volumes require every attached instance to be known stopped before idle. ALB redirects and fixed responses count as activity without targets. Check dependencies and infrequent workloads before acting.

## Costs and limitations

| Source | Meaning |
| --- | --- |
| `actual` | Cost Explorer total over 14 UTC days projected as total / 14 × 30 |
| `estimate` | On-demand list price or fixed fee × capacity/observed usage |
| `none` | No price/data, unsupported pricing, or disabled cost; see `note` |

`--cost auto` tries actual EC2 Compute cost, then estimates resources without actual cost. The [resource-cost API requires an EC2 Compute filter](https://docs.aws.amazon.com/aws-cost-management/latest/APIReference/API_GetCostAndUsageWithResources.html). Resource-level data must be enabled in the payer account and covers 14 days; data may be delayed. Failures produce one fallback warning. Other resource types use estimates. This is a projection, not a future invoice.

Each paginated Cost Explorer request to the primary billing view [costs $0.01](https://aws.amazon.com/aws-cost-management/aws-cost-explorer/pricing/). The scan reports its request count. Use `--cost estimate` to avoid Cost Explorer calls, or `--cost none` to skip cost calculations. CloudWatch GetMetricData also has usage charges; metrics are batched up to 500 queries. Use `--no-metrics` to disable these calls independently. Metric-dependent verdicts stay unknown without evidence; inventory, local checks and structural verdicts still run. `--cost none --no-metrics` disables both paid analysis paths.

Estimates exclude Reserved Instances, Savings Plans, Spot discounts, free tier, and data transfer. Capacity uses 730 hours/month; measured usage is normalized to 30 days. When multiple dimensions match, the lowest positive on-demand rate is selected, so licensing and tiered prices can differ from the bill. Read each cost `note`:

- EBS excludes extra IOPS/throughput; snapshots assume full source volume size.
- NAT data processing and ELB capacity units are excluded.
- RDS excludes backups and I/O. Stopped instances exclude compute and retain applicable storage; RDS can automatically restart them after up to seven days, so this stopped run rate is not a month-long forecast. Cluster storage is unpriced; member instances are priced separately.
- EKS uses the fixed standard control-plane fee, excluding nodes and extended support. Route53 includes the zone fee only.
- Provisioned DynamoDB excludes storage. On-demand DynamoDB, SQS, and CloudFront are unpriced. Lambda uses requests and duration.
- S3 estimates Standard storage only. Empty buckets can be idle; nonempty buckets remain `unknown` because access metrics are not enabled by default.
- IAM, ACM, and security groups have no estimator.

## Findings

| Severity | Checks |
| --- | --- |
| High | `sg.open_admin_port`, `rds.public`, `s3.public_access_not_blocked`, `iam.user_no_mfa`, `acm.expiring` |
| Medium | `ebs.unencrypted`, `rds.unencrypted`, `rds.no_backups`, `ec2.imdsv1`, `iam.access_key_old`, `cloudfront.http_allowed`, `cloudfront.old_tls`, `eks.public_endpoint`, `lambda.deprecated_runtime` |
| Low | `ebs.gp2`, `sqs.unencrypted`, `acm.unused` |
| Medium / low | `usage.idle`, `usage.orphan`: medium at $50/month or more, otherwise low |

Certificates in use are checked within 30 days of expiry; active keys older than 90 days are flagged. Aurora member backup settings are excluded from the instance backup check. Missing security facts are not assumed disabled. Deprecated runtimes were checked against the [AWS runtime table](https://docs.aws.amazon.com/lambda/latest/dg/lambda-runtimes.html) on November 15, 2026; the list needs periodic updates.

## Read-only IAM policy

This policy lists operations used by the implementation, including tags and auxiliary joins. `Resource: "*"` supports inventory APIs without resource-level permissions. Organization/resource policies may impose further restrictions. Cost Explorer permissions are optional with `--cost estimate`; Pricing permissions are optional with `--cost none`.

```json
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Action": [
      "sts:GetCallerIdentity",
      "ec2:DescribeRegions", "ec2:DescribeInstances", "ec2:DescribeVolumes",
      "ec2:DescribeSnapshots", "ec2:DescribeImages", "ec2:DescribeAddresses",
      "ec2:DescribeNatGateways", "ec2:DescribeSecurityGroups",
      "elasticloadbalancing:DescribeLoadBalancers", "elasticloadbalancing:DescribeTargetGroups",
      "elasticloadbalancing:DescribeTargetHealth", "elasticloadbalancing:DescribeTags",
      "rds:DescribeDBInstances", "rds:DescribeDBClusters",
      "elasticache:DescribeCacheClusters", "elasticache:ListTagsForResource",
      "dynamodb:ListTables", "dynamodb:DescribeTable", "dynamodb:ListTagsOfResource",
      "s3:ListAllMyBuckets", "s3:GetBucketLocation", "s3:GetBucketTagging", "s3:GetBucketPublicAccessBlock",
      "sqs:ListQueues", "sqs:GetQueueAttributes", "sqs:ListQueueTags",
      "lambda:ListFunctions", "lambda:ListTags",
      "eks:ListClusters", "eks:DescribeCluster", "eks:ListNodegroups", "eks:ListFargateProfiles",
      "cloudfront:ListDistributions", "cloudfront:ListTagsForResource",
      "iam:ListUsers", "iam:GetLoginProfile", "iam:ListMFADevices", "iam:ListAccessKeys",
      "iam:ListUserTags", "iam:ListGroups", "iam:ListRoles", "iam:GetRole", "iam:ListRoleTags",
      "acm:ListCertificates", "acm:DescribeCertificate", "acm:ListTagsForCertificate",
      "route53:ListHostedZones", "route53:ListTagsForResource",
      "cloudwatch:GetMetricData", "pricing:GetProducts", "ce:GetCostAndUsageWithResources"
    ],
    "Resource": "*"
  }]
}
```

## Development

```sh
go build ./...
go vet ./...
go test -race ./...
```

Tests use fake AWS clients and never call live services. Release tags run GoReleaser v2 through GitHub Actions. MIT licensed; see [LICENSE](LICENSE).
