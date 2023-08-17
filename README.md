# lookr - A Aws CLI

The `lookr` CLI is a tool that allows you to query information about various Amazon Web Services (AWS) services across different regions. It leverages the official AWS SDK for Go to interact with these services and presents the results in a tabular format.

## Installation

To install the `lookr` CLI, follow these steps:

1. Ensure you have Go installed on your machine. If not, you can download it from https://golang.org/dl/
2. Clone this repository or download the ZIP file.
3. Navigate to the CLI directory using your terminal.
4. Run the following command to build the CLI:

```bash
go build -o lookr cmd/lookr/main.go
```

5. Now you can run the CLI using `./lookr`.

## Commands

The `lookr` CLI provides the following commands to query information about various AWS services:

- `ec2`: Query information about EC2 instances.
- `rds`: Query information about RDS databases.
- `sqs`: Query information about Amazon SQS queues.
- `lambda`: Query information about AWS Lambda functions.
- `iam`: Query information about IAM groups, users, and roles.
- `ebs`: Query information about Amazon EBS volumes.
- `acm`: Query information about AWS Certificate Manager certificates.
- `cloudfront`: Query information about Amazon CloudFront distributions.
- `elasticache`: Query information about Amazon ElastiCache clusters.
- `dynamodb`: Query information about Amazon DynamoDB tables.

## Usage

To use the `lookr` CLI and query information about a specific service, execute the following command:

```shell
./lookr <command>
``````

Replace <command> with the desired service name. For example, to query information about EC2 instances, use:

```shell
./lookr ec2
``````

The CLI will display the results in a tabular format, showing details about the service in the current region and other configured regions.

Contribution
If you'd like to contribute to this project, feel free to fork the repository, create a branch with your changes, and submit a pull request.

License
This project is licensed under the MIT License - see the LICENSE file for details.
