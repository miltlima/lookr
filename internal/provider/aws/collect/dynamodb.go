package collect

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"lookr/internal/model"
)

type tableAPI interface {
	ListTables(context.Context, *dynamodb.ListTablesInput, ...func(*dynamodb.Options)) (*dynamodb.ListTablesOutput, error)
	DescribeTable(context.Context, *dynamodb.DescribeTableInput, ...func(*dynamodb.Options)) (*dynamodb.DescribeTableOutput, error)
}

func collectTables(ctx context.Context, c tableAPI, s Scope) ([]model.Resource, error) {
	var result []model.Resource
	in := &dynamodb.ListTablesInput{}
	for {
		o, e := c.ListTables(ctx, in)
		if e != nil {
			return result, e
		}
		for _, id := range o.TableNames {
			d, e := c.DescribeTable(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(id)})
			if e != nil {
				return result, e
			}
			if d.Table == nil {
				return result, fmt.Errorf("table %s: missing description", id)
			}
			v := d.Table
			billing := "PROVISIONED"
			if v.BillingModeSummary != nil {
				billing = string(v.BillingModeSummary.BillingMode)
			}
			read, write := 0, 0
			if v.ProvisionedThroughput != nil {
				read = int(aws.ToInt64(v.ProvisionedThroughput.ReadCapacityUnits))
				write = int(aws.ToInt64(v.ProvisionedThroughput.WriteCapacityUnits))
			}
			class := "STANDARD"
			if v.TableClassSummary != nil {
				class = string(v.TableClassSummary.TableClass)
			}
			r := resource(s, "aws.dynamodb.table", "dynamodb", id, "", nil, map[string]any{"status": string(v.TableStatus), "billing_mode": billing, "read_capacity": read, "write_capacity": write, "item_count": int(aws.ToInt64(v.ItemCount)), "size_bytes": int(aws.ToInt64(v.TableSizeBytes)), "table_class": class})
			r.ARN = aws.ToString(v.TableArn)
			r.CreatedAt = v.CreationDateTime
			if tc, ok := c.(tableTagAPI); ok {
				tags := map[string]string{}
				tagIn := &dynamodb.ListTagsOfResourceInput{ResourceArn: aws.String(r.ARN)}
				for {
					tagged, err := tc.ListTagsOfResource(ctx, tagIn)
					if err != nil {
						result = append(result, r)
						return result, err
					}
					for _, t := range tagged.Tags {
						tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
					}
					if aws.ToString(tagged.NextToken) == "" {
						break
					}
					tagIn.NextToken = tagged.NextToken
				}
				r.Tags = tags
				if tags["Name"] != "" {
					r.Name = tags["Name"]
				}
			}
			result = append(result, r)
		}
		if aws.ToString(o.LastEvaluatedTableName) == "" {
			break
		}
		in.ExclusiveStartTableName = o.LastEvaluatedTableName
	}
	return result, nil
}

type tableTagAPI interface {
	ListTagsOfResource(context.Context, *dynamodb.ListTagsOfResourceInput, ...func(*dynamodb.Options)) (*dynamodb.ListTagsOfResourceOutput, error)
}
